/*
  甲骨文云API文档
  https://docs.oracle.com/en-us/iaas/api/#/en/iaas/20160918/

  实例:
  https://docs.oracle.com/en-us/iaas/api/#/en/iaas/20160918/Instance/
  VCN:
  https://docs.oracle.com/en-us/iaas/api/#/en/iaas/20160918/Vcn/
  Subnet:
  https://docs.oracle.com/en-us/iaas/api/#/en/iaas/20160918/Subnet/
  VNIC:
  https://docs.oracle.com/en-us/iaas/api/#/en/iaas/20160918/Vnic/
  VnicAttachment:
  https://docs.oracle.com/en-us/iaas/api/#/en/iaas/20160918/VnicAttachment/
  私有IP
  https://docs.oracle.com/en-us/iaas/api/#/en/iaas/20160918/PrivateIp/
  公共IP
  https://docs.oracle.com/en-us/iaas/api/#/en/iaas/20160918/PublicIp/

  获取可用性域
  https://docs.oracle.com/en-us/iaas/api/#/en/identity/20160918/AvailabilityDomain/ListAvailabilityDomains
*/
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/oracle/oci-go-sdk/v54/common"
	"github.com/oracle/oci-go-sdk/v54/core"
	"github.com/oracle/oci-go-sdk/v54/example/helpers"
	"github.com/oracle/oci-go-sdk/v54/identity"
	"gopkg.in/ini.v1"
)

const (
	defConfigFilePath = "./oci-help.ini"
	IPsFilePrefix     = "IPs"
)

var (
	configFilePath      string
	provider            common.ConfigurationProvider
	computeClient       core.ComputeClient
	networkClient       core.VirtualNetworkClient
	storageClient       core.BlockstorageClient
	identityClient      identity.IdentityClient
	ctx                 context.Context
	oracleSections      []*ini.Section
	oracleSection       *ini.Section
	oracleSectionName   string
	oracle              Oracle
	instanceBaseSection *ini.Section
	instance            Instance
	proxy               string
	token               string
	chat_id             string
	sendMessageUrl      string
	editMessageUrl      string
	EACH                bool
	availabilityDomains []identity.AvailabilityDomain
)

type Oracle struct {
	User         string `ini:"user"`
	Fingerprint  string `ini:"fingerprint"`
	Tenancy      string `ini:"tenancy"`
	Region       string `ini:"region"`
	Key_file     string `ini:"key_file"`
	Key_password string `ini:"key_password"`
}

type Instance struct {
	AvailabilityDomain     string  `ini:"availabilityDomain"`
	SSH_Public_Key         string  `ini:"ssh_authorized_key"`
	VcnDisplayName         string  `ini:"vcnDisplayName"`
	SubnetDisplayName      string  `ini:"subnetDisplayName"`
	Shape                  string  `ini:"shape"`
	OperatingSystem        string  `ini:"OperatingSystem"`
	OperatingSystemVersion string  `ini:"OperatingSystemVersion"`
	InstanceDisplayName    string  `ini:"instanceDisplayName"`
	Ocpus                  float32 `ini:"cpus"`
	MemoryInGBs            float32 `ini:"memoryInGBs"`
	BootVolumeSizeInGBs    int64   `ini:"bootVolumeSizeInGBs"`
	Sum                    int32   `ini:"sum"`
	Each                   int32   `ini:"each"`
	Retry                  int32   `ini:"retry"`
	CloudInit              string  `ini:"cloud-init"`
	MinTime                int32   `ini:"minTime"`
	MaxTime                int32   `ini:"maxTime"`
}

type Message struct {
	OK          bool `json:"ok"`
	Result      `json:"result"`
	ErrorCode   int    `json:"error_code"`
	Description string `json:"description"`
}
type Result struct {
	MessageId int `json:"message_id"`
}

func main() {
	flag.StringVar(&configFilePath, "config", defConfigFilePath, "配置文件路径")
	flag.StringVar(&configFilePath, "c", defConfigFilePath, "配置文件路径")
	flag.Parse()

	cfg, err := ini.Load(configFilePath)
	helpers.FatalIfError(err)
	defSec := cfg.Section(ini.DefaultSection)
	proxy = defSec.Key("proxy").Value()
	token = defSec.Key("token").Value()
	chat_id = defSec.Key("chat_id").Value()
	if defSec.HasKey("EACH") {
		EACH, _ = defSec.Key("EACH").Bool()
	} else {
		EACH = true
	}
	sendMessageUrl = "https://api.telegram.org/bot" + token + "/sendMessage"
	editMessageUrl = "https://api.telegram.org/bot" + token + "/editMessageText"
	rand.Seed(time.Now().UnixNano())

	sections := cfg.Sections()
	oracleSections = []*ini.Section{}
	for _, sec := range sections {
		if len(sec.ParentKeys()) == 0 {
			user := sec.Key("user").Value()
			fingerprint := sec.Key("fingerprint").Value()
			tenancy := sec.Key("tenancy").Value()
			region := sec.Key("region").Value()
			key_file := sec.Key("key_file").Value()
			if user != "" && fingerprint != "" && tenancy != "" && region != "" && key_file != "" {
				oracleSections = append(oracleSections, sec)
			}
		}
	}
	if len(oracleSections) == 0 {
		fmt.Printf("\033[1;31m未找到正确的配置信息, 请参考链接文档配置相关信息。链接: https://github.com/lemoex/oci-help\033[0m\n")
		return
	}
	instanceBaseSection = cfg.Section("INSTANCE")

	listOracleAccount()
}

func listOracleAccount() {
	if len(oracleSections) == 1 {
		oracleSection = oracleSections[0]
	} else {
		fmt.Printf("\n\033[1;32m%s\033[0m\n\n", "欢迎使用甲骨文实例管理工具")
		w := new(tabwriter.Writer)
		w.Init(os.Stdout, 4, 8, 1, '\t', 0)
		fmt.Fprintf(w, "%s\t%s\t\n", "序号", "账号")
		for i, section := range oracleSections {
			fmt.Fprintf(w, "%d\t%s\t\n", i+1, section.Name())
		}
		w.Flush()
		fmt.Printf("\n")
		var input string
		var index int
		for {
			fmt.Print("请输入账号对应的序号进入相关操作: ")
			_, err := fmt.Scanln(&input)
			if err != nil {
				return
			}
			if strings.EqualFold(input, "oci") {
				multiBatchLaunchInstances()
				listOracleAccount()
				return
			} else if strings.EqualFold(input, "ip") {
				multiBatchListInstancesIp()
				listOracleAccount()
				return
			}
			index, _ = strconv.Atoi(input)
			if 0 < index && index <= len(oracleSections) {
				break
			} else {
				index = 0
				input = ""
				fmt.Printf("\033[1;31m错误! 请输入正确的序号\033[0m\n")
			}
		}
		oracleSection = oracleSections[index-1]
	}

	var err error
	ctx = context.Background()
	err = initVar(oracleSection)
	if err != nil {
		return
	}
	// 获取可用性域
	fmt.Println("正在获取可用性域...")
	availabilityDomains, err = ListAvailabilityDomains()
	if err != nil {
		printlnErr("获取可用性域失败", err.Error())
		return
	}

	showMainMenu()
}

func initVar(oracleSec *ini.Section) (err error) {
	oracleSectionName = oracleSec.Name()
	oracle = Oracle{}
	err = oracleSec.MapTo(&oracle)
	if err != nil {
		printlnErr("解析账号相关参数失败", err.Error())
		return
	}
	provider, err = getProvider(oracle)
	if err != nil {
		printlnErr("获取 Provider 失败", err.Error())
		return
	}

	computeClient, err = core.NewComputeClientWithConfigurationProvider(provider)
	if err != nil {
		printlnErr("创建 ComputeClient 失败", err.Error())
		return
	}
	setProxyOrNot(&computeClient.BaseClient)
	networkClient, err = core.NewVirtualNetworkClientWithConfigurationProvider(provider)
	if err != nil {
		printlnErr("创建 VirtualNetworkClient 失败", err.Error())
		return
	}
	setProxyOrNot(&networkClient.BaseClient)
	storageClient, err = core.NewBlockstorageClientWithConfigurationProvider(provider)
	if err != nil {
		printlnErr("创建 BlockstorageClient 失败", err.Error())
		return
	}
	setProxyOrNot(&storageClient.BaseClient)
	identityClient, err = identity.NewIdentityClientWithConfigurationProvider(provider)
	if err != nil {
		printlnErr("创建 IdentityClient 失败", err.Error())
		return
	}
	setProxyOrNot(&identityClient.BaseClient)
	return
}

func showMainMenu() {
	fmt.Printf("\n\033[1;32m欢迎使用甲骨文实例管理工具\033[0m \n(当前账号: %s)\n\n", oracleSection.Name())
	fmt.Printf("\033[1;36m%s\033[0m %s\n", "1.", "查看实例")
	fmt.Printf("\033[1;36m%s\033[0m %s\n", "2.", "创建实例")
	fmt.Printf("\033[1;36m%s\033[0m %s\n", "3.", "管理引导卷")
	fmt.Print("\n请输入序号进入相关操作: ")
	var input string
	var num int
	fmt.Scanln(&input)
	if strings.EqualFold(input, "oci") {
		batchLaunchInstances(oracleSection)
		showMainMenu()
		return
	} else if strings.EqualFold(input, "ip") {
		batchListInstancesIp(oracleSection)
		showMainMenu()
		return
	}
	num, _ = strconv.Atoi(input)
	switch num {
	case 1:
		listInstances()
	case 2:
		listLaunchInstanceTemplates()
	case 3:
		listBootVolumes()
	default:
		if len(oracleSections) > 1 {
			listOracleAccount()
		}
	}
}

func listInstances() {
	fmt.Println("正在获取实例数据...")
	instances, err := ListInstances(ctx, computeClient)
	if err != nil {
		printlnErr("获取失败, 回车返回上一级菜单.", err.Error())
		fmt.Scanln()
		showMainMenu()
		return
	}
	if len(instances) == 0 {
		fmt.Printf("\033[1;32m实例为空, 回车返回上一级菜单.\033[0m")
		fmt.Scanln()
		showMainMenu()
		return
	}
	fmt.Printf("\n\033[1;32m实例信息\033[0m \n(当前账号: %s)\n\n", oracleSection.Name())
	w := new(tabwriter.Writer)
	w.Init(os.Stdout, 4, 8, 1, '\t', 0)
	fmt.Fprintf(w, "%s\t%s\t%s\t%s\t\n", "序号", "名称", "状态　　", "配置")
	//fmt.Printf("%-5s %-28s %-18s %-20s\n", "序号", "名称", "公共IP", "配置")
	for i, ins := range instances {
		// 获取实例公共IP
		/*
			var strIps string
			ips, err := getInstancePublicIps(ctx, computeClient, networkClient, ins.Id)
			if err != nil {
				strIps = err.Error()
			} else {
				strIps = strings.Join(ips, ",")
			}
		*/
		//fmt.Printf("%-7d %-30s %-20s %-20s\n", i+1, *ins.DisplayName, strIps, *ins.Shape)

		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t\n", i+1, *ins.DisplayName, getInstanceState(ins.LifecycleState), *ins.Shape)
	}
	w.Flush()
	fmt.Println("--------------------")
	fmt.Printf("\n\033[1;32ma: %s   b: %s   c: %s   d: %s\033[0m\n", "启动全部", "停止全部", "重启全部", "终止全部")
	var input string
	var index int
	for {
		fmt.Print("请输入序号查看实例详细信息: ")
		_, err := fmt.Scanln(&input)
		if err != nil {
			showMainMenu()
			return
		}
		switch input {
		case "a":
			fmt.Printf("确定启动全部实例？(输入 y 并回车): ")
			var input string
			fmt.Scanln(&input)
			if strings.EqualFold(input, "y") {
				for _, ins := range instances {
					_, err := instanceAction(ins.Id, core.InstanceActionActionStart)
					if err != nil {
						fmt.Printf("\033[1;31m实例 %s 启动失败.\033[0m %s\n", *ins.DisplayName, err.Error())
					} else {
						fmt.Printf("\033[1;32m实例 %s 启动成功.\033[0m\n", *ins.DisplayName)
					}
				}
			} else {
				continue
			}
			time.Sleep(1 * time.Second)
			listInstances()
			return
		case "b":
			fmt.Printf("确定停止全部实例？(输入 y 并回车): ")
			var input string
			fmt.Scanln(&input)
			if strings.EqualFold(input, "y") {
				for _, ins := range instances {
					_, err := instanceAction(ins.Id, core.InstanceActionActionSoftstop)
					if err != nil {
						fmt.Printf("\033[1;31m实例 %s 停止失败.\033[0m %s\n", *ins.DisplayName, err.Error())
					} else {
						fmt.Printf("\033[1;32m实例 %s 停止成功.\033[0m\n", *ins.DisplayName)
					}
				}
			} else {
				continue
			}
			time.Sleep(1 * time.Second)
			listInstances()
			return
		case "c":
			fmt.Printf("确定重启全部实例？(输入 y 并回车): ")
			var input string
			fmt.Scanln(&input)
			if strings.EqualFold(input, "y") {
				for _, ins := range instances {
					_, err := instanceAction(ins.Id, core.InstanceActionActionSoftreset)
					if err != nil {
						fmt.Printf("\033[1;31m实例 %s 重启失败.\033[0m %s\n", *ins.DisplayName, err.Error())
					} else {
						fmt.Printf("\033[1;32m实例 %s 重启成功.\033[0m\n", *ins.DisplayName)
					}
				}
			} else {
				continue
			}
			time.Sleep(1 * time.Second)
			listInstances()
			return
		case "d":
			fmt.Printf("确定终止全部实例？(输入 y 并回车): ")
			var input string
			fmt.Scanln(&input)
			if strings.EqualFold(input, "y") {
				for _, ins := range instances {
					err := terminateInstance(ins.Id)
					if err != nil {
						fmt.Printf("\033[1;31m实例 %s 终止失败.\033[0m %s\n", *ins.DisplayName, err.Error())
					} else {
						fmt.Printf("\033[1;32m实例 %s 终止成功.\033[0m\n", *ins.DisplayName)
					}
				}
			} else {
				continue
			}
			time.Sleep(1 * time.Second)
			listInstances()
			return
		}
		index, _ = strconv.Atoi(input)
		if 0 < index && index <= len(instances) {
			break
		} else {
			input = ""
			index = 0
			fmt.Printf("\033[1;31m错误! 请输入正确的序号\033[0m\n")
		}
	}
	instanceDetails(instances[index-1].Id)
}

func instanceDetails(instanceId *string) {
	for {
		fmt.Println("正在获取实例详细信息...")
		instance, err := getInstance(instanceId)
		if err != nil {
			fmt.Printf("\033[1;31m获取实例详细信息失败, 回车返回上一级菜单.\033[0m")
			fmt.Scanln()
			listInstances()
			return
		}
		vnics, err := getInstanceVnics(instanceId)
		if err != nil {
			fmt.Printf("\033[1;31m获取实例VNIC失败, 回车返回上一级菜单.\033[0m")
			fmt.Scanln()
			listInstances()
			return
		}
		var publicIps = make([]string, 0)
		var strPublicIps string
		if err != nil {
			strPublicIps = err.Error()
		} else {
			for _, vnic := range vnics {
				if vnic.PublicIp != nil {
					publicIps = append(publicIps, *vnic.PublicIp)
				}
			}
			strPublicIps = strings.Join(publicIps, ",")
		}

		fmt.Printf("\n\033[1;32m实例详细信息\033[0m \n(当前账号: %s)\n\n", oracleSection.Name())
		fmt.Println("--------------------")
		fmt.Printf("名称: %s\n", *instance.DisplayName)
		fmt.Printf("状态: %s\n", getInstanceState(instance.LifecycleState))
		fmt.Printf("公共IP: %s\n", strPublicIps)
		fmt.Printf("可用性域: %s\n", *instance.AvailabilityDomain)
		fmt.Printf("配置: %s\n", *instance.Shape)
		fmt.Printf("OCPU计数: %g\n", *instance.ShapeConfig.Ocpus)
		fmt.Printf("网络带宽(Gbps): %g\n", *instance.ShapeConfig.NetworkingBandwidthInGbps)
		fmt.Printf("内存(GB): %g\n", *instance.ShapeConfig.MemoryInGBs)
		fmt.Println("--------------------")
		fmt.Printf("\n\033[1;32m1: %s   2: %s   3: %s   4: %s   5: %s\033[0m\n", "启动", "停止", "重启", "终止", "更换公共IP")
		var input string
		var num int
		fmt.Print("\n请输入需要执行操作的序号: ")
		fmt.Scanln(&input)
		num, _ = strconv.Atoi(input)
		switch num {
		case 1:
			_, err := instanceAction(instance.Id, core.InstanceActionActionStart)
			if err != nil {
				fmt.Printf("\033[1;31m启动实例失败.\033[0m %s\n", err.Error())
			} else {
				fmt.Printf("\033[1;32m正在启动实例, 请稍后查看实例状态\033[0m\n")
			}
			time.Sleep(3 * time.Second)

		case 2:
			_, err := instanceAction(instance.Id, core.InstanceActionActionSoftstop)
			if err != nil {
				fmt.Printf("\033[1;31m停止实例失败.\033[0m %s\n", err.Error())
			} else {
				fmt.Printf("\033[1;32m正在停止实例, 请稍后查看实例状态\033[0m\n")
			}
			time.Sleep(3 * time.Second)

		case 3:
			_, err := instanceAction(instance.Id, core.InstanceActionActionSoftreset)
			if err != nil {
				fmt.Printf("\033[1;31m重启实例失败.\033[0m %s\n", err.Error())
			} else {
				fmt.Printf("\033[1;32m正在重启实例, 请稍后查看实例状态\033[0m\n")
			}
			time.Sleep(3 * time.Second)

		case 4:
			fmt.Printf("确定终止实例？(输入 y 并回车): ")
			var input string
			fmt.Scanln(&input)
			if strings.EqualFold(input, "y") {
				err := terminateInstance(instance.Id)
				if err != nil {
					fmt.Printf("\033[1;31m终止实例失败.\033[0m %s\n", err.Error())
				} else {
					fmt.Printf("\033[1;32m正在终止实例, 请稍后查看实例状态\033[0m\n")
				}
				time.Sleep(3 * time.Second)
			}

		case 5:
			if len(vnics) == 0 {
				fmt.Printf("\033[1;31m实例已终止或获取实例VNIC失败，请稍后重试.\033[0m\n")
				break
			}
			fmt.Printf("将删除当前公共IP并创建一个新的公共IP。确定更换实例公共IP？(输入 y 并回车): ")
			var input string
			fmt.Scanln(&input)
			if strings.EqualFold(input, "y") {
				publicIp, err := changePublicIp(vnics)
				if err != nil {
					fmt.Printf("\033[1;31m更换实例公共IP失败.\033[0m %s\n", err.Error())
				} else {
					fmt.Printf("\033[1;32m更换实例公共IP成功, 实例公共IP: \033[0m%s\n", *publicIp.IpAddress)
				}
				time.Sleep(3 * time.Second)
			}

		default:
			listInstances()
			return
		}
	}
}

func listBootVolumes() {
	var bootVolumes []core.BootVolume
	var wg sync.WaitGroup
	for _, ad := range availabilityDomains {
		wg.Add(1)
		go func(adName *string) {
			defer wg.Done()
			volumes, err := getBootVolumes(adName)
			if err != nil {
				printlnErr("获取引导卷失败", err.Error())
			} else {
				bootVolumes = append(bootVolumes, volumes...)
			}
		}(ad.Name)
	}
	wg.Wait()

	fmt.Printf("\n\033[1;32m引导卷\033[0m \n(当前账号: %s)\n\n", oracleSection.Name())
	w := new(tabwriter.Writer)
	w.Init(os.Stdout, 4, 8, 1, '\t', 0)
	fmt.Fprintf(w, "%s\t%s\t%s\t%s\t\n", "序号", "名称", "状态　　", "大小(GB)")
	for i, volume := range bootVolumes {
		fmt.Fprintf(w, "%d\t%s\t%s\t%d\t\n", i+1, *volume.DisplayName, getBootVolumeState(volume.LifecycleState), *volume.SizeInGBs)
	}
	w.Flush()
	fmt.Printf("\n")
	var input string
	var index int
	for {
		fmt.Print("请输入序号查看引导卷详细信息: ")
		_, err := fmt.Scanln(&input)
		if err != nil {
			showMainMenu()
			return
		}
		index, _ = strconv.Atoi(input)
		if 0 < index && index <= len(bootVolumes) {
			break
		} else {
			input = ""
			index = 0
			fmt.Printf("\033[1;31m错误! 请输入正确的序号\033[0m\n")
		}
	}
	bootvolumeDetails(bootVolumes[index-1].Id)
}

func bootvolumeDetails(bootVolumeId *string) {
	for {
		fmt.Println("正在获取引导卷详细信息...")
		bootVolume, err := getBootVolume(bootVolumeId)
		if err != nil {
			fmt.Printf("\033[1;31m获取引导卷详细信息失败, 回车返回上一级菜单.\033[0m")
			fmt.Scanln()
			listBootVolumes()
			return
		}

		attachments, err := listBootVolumeAttachments(bootVolume.AvailabilityDomain, bootVolume.CompartmentId, bootVolume.Id)
		attachIns := make([]string, 0)
		if err != nil {
			attachIns = append(attachIns, err.Error())
		} else {
			for _, attachment := range attachments {
				ins, err := getInstance(attachment.InstanceId)
				if err != nil {
					attachIns = append(attachIns, err.Error())
				} else {
					attachIns = append(attachIns, *ins.DisplayName)
				}
			}
		}

		var performance string
		switch *bootVolume.VpusPerGB {
		case 10:
			performance = fmt.Sprintf("均衡 (VPU:%d)", *bootVolume.VpusPerGB)
		case 20:
			performance = fmt.Sprintf("性能较高 (VPU:%d)", *bootVolume.VpusPerGB)
		default:
			performance = fmt.Sprintf("UHP (VPU:%d)", *bootVolume.VpusPerGB)
		}

		fmt.Printf("\n\033[1;32m引导卷详细信息\033[0m \n(当前账号: %s)\n\n", oracleSection.Name())
		fmt.Println("--------------------")
		fmt.Printf("名称: %s\n", *bootVolume.DisplayName)
		fmt.Printf("状态: %s\n", getBootVolumeState(bootVolume.LifecycleState))
		fmt.Printf("可用性域: %s\n", *bootVolume.AvailabilityDomain)
		fmt.Printf("大小(GB): %d\n", *bootVolume.SizeInGBs)
		fmt.Printf("性能: %s\n", performance)
		fmt.Printf("附加的实例: %s\n", strings.Join(attachIns, ","))
		fmt.Println("--------------------")
		fmt.Printf("\n\033[1;32m1: %s   2: %s   3: %s   4: %s\033[0m\n", "修改性能", "修改大小", "分离引导卷", "终止引导卷")
		var input string
		var num int
		fmt.Print("\n请输入需要执行操作的序号: ")
		fmt.Scanln(&input)
		num, _ = strconv.Atoi(input)
		switch num {
		case 1:
			fmt.Printf("修改引导卷性能, 请输入 (1: 均衡; 2: 性能较高): ")
			var input string
			fmt.Scanln(&input)
			if input == "1" {
				_, err := updateBootVolume(bootVolume.Id, nil, common.Int64(10))
				if err != nil {
					fmt.Printf("\033[1;31m修改引导卷性能失败.\033[0m %s\n", err.Error())
				} else {
					fmt.Printf("\033[1;32m修改引导卷性能成功, 请稍后查看引导卷状态\033[0m\n")
				}
			} else if input == "2" {
				_, err := updateBootVolume(bootVolume.Id, nil, common.Int64(20))
				if err != nil {
					fmt.Printf("\033[1;31m修改引导卷性能失败.\033[0m %s\n", err.Error())
				} else {
					fmt.Printf("\033[1;32m修改引导卷性能成功, 请稍后查看引导卷信息\033[0m\n")
				}
			} else {
				fmt.Printf("\033[1;31m输入错误.\033[0m\n")
			}
			time.Sleep(1 * time.Second)

		case 2:
			fmt.Printf("修改引导卷大小, 请输入 (例如修改为50GB, 输入50): ")
			var input string
			var sizeInGBs int64
			fmt.Scanln(&input)
			sizeInGBs, _ = strconv.ParseInt(input, 10, 64)
			if sizeInGBs > 0 {
				_, err := updateBootVolume(bootVolume.Id, &sizeInGBs, nil)
				if err != nil {
					fmt.Printf("\033[1;31m修改引导卷大小失败.\033[0m %s\n", err.Error())
				} else {
					fmt.Printf("\033[1;32m修改引导卷大小成功, 请稍后查看引导卷信息\033[0m\n")
				}
			} else {
				fmt.Printf("\033[1;31m输入错误.\033[0m\n")
			}
			time.Sleep(1 * time.Second)

		case 3:
			fmt.Printf("确定分离引导卷？(输入 y 并回车): ")
			var input string
			fmt.Scanln(&input)
			if strings.EqualFold(input, "y") {
				for _, attachment := range attachments {
					_, err := detachBootVolume(attachment.Id)
					if err != nil {
						fmt.Printf("\033[1;31m分离引导卷失败.\033[0m %s\n", err.Error())
					} else {
						fmt.Printf("\033[1;32m分离引导卷成功, 请稍后查看引导卷信息\033[0m\n")
					}
				}
			}
			time.Sleep(1 * time.Second)

		case 4:
			fmt.Printf("确定终止引导卷？(输入 y 并回车): ")
			var input string
			fmt.Scanln(&input)
			if strings.EqualFold(input, "y") {
				_, err := deleteBootVolume(bootVolume.Id)
				if err != nil {
					fmt.Printf("\033[1;31m终止引导卷失败.\033[0m %s\n", err.Error())
				} else {
					fmt.Printf("\033[1;32m终止引导卷成功, 请稍后查看引导卷信息\033[0m\n")
				}

			}
			time.Sleep(1 * time.Second)

		default:
			listBootVolumes()
			return
		}
	}
}

func listLaunchInstanceTemplates() {
	var instanceSections []*ini.Section
	instanceSections = append(instanceSections, instanceBaseSection.ChildSections()...)
	instanceSections = append(instanceSections, oracleSection.ChildSections()...)
	if len(instanceSections) == 0 {
		fmt.Printf("\033[1;31m未找到实例模版, 回车返回上一级菜单.\033[0m")
		fmt.Scanln()
		showMainMenu()
		return
	}

	for {
		fmt.Printf("\n\033[1;32m选择对应的实例模版开始创建实例\033[0m \n(当前账号: %s)\n\n", oracleSectionName)
		w := new(tabwriter.Writer)
		w.Init(os.Stdout, 4, 8, 1, '\t', 0)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t\n", "序号", "配置", "CPU个数", "内存(GB)")
		for i, instanceSec := range instanceSections {
			cpu := instanceSec.Key("cpus").Value()
			if cpu == "" {
				cpu = "-"
			}
			memory := instanceSec.Key("memoryInGBs").Value()
			if memory == "" {
				memory = "-"
			}
			fmt.Fprintf(w, "%d\t%s\t%s\t%s\t\n", i+1, instanceSec.Key("shape").Value(), cpu, memory)
		}
		w.Flush()
		fmt.Printf("\n")
		var input string
		var index int
		for {
			fmt.Print("请输入需要创建的实例的序号: ")
			_, err := fmt.Scanln(&input)
			if err != nil {
				showMainMenu()
				return
			}
			index, _ = strconv.Atoi(input)
			if 0 < index && index <= len(instanceSections) {
				break
			} else {
				input = ""
				index = 0
				fmt.Printf("\033[1;31m错误! 请输入正确的序号\033[0m\n")
			}
		}

		instanceSection := instanceSections[index-1]
		instance = Instance{}
		err := instanceSection.MapTo(&instance)
		if err != nil {
			printlnErr("解析实例模版参数失败", err.Error())
			continue
		}

		LaunchInstances(availabilityDomains)
	}

}

func multiBatchLaunchInstances() {
	for _, sec := range oracleSections {
		var err error
		err = initVar(sec)
		if err != nil {
			continue
		}
		// 获取可用性域
		availabilityDomains, err = ListAvailabilityDomains()
		if err != nil {
			printlnErr("获取可用性域失败", err.Error())
			continue
		}
		batchLaunchInstances(sec)
	}
}

func batchLaunchInstances(oracleSec *ini.Section) {
	var instanceSections []*ini.Section
	instanceSections = append(instanceSections, instanceBaseSection.ChildSections()...)
	instanceSections = append(instanceSections, oracleSec.ChildSections()...)
	if len(instanceSections) == 0 {
		return
	}

	printf("\033[1;36m[%s] 开始创建\033[0m\n", oracleSectionName)
	var SUM, NUM int32 = 0, 0
	sendMessage(fmt.Sprintf("[%s]", oracleSectionName), "开始创建")

	for _, instanceSec := range instanceSections {
		instance = Instance{}
		err := instanceSec.MapTo(&instance)
		if err != nil {
			printlnErr("解析实例模版参数失败", err.Error())
			continue
		}

		sum, num := LaunchInstances(availabilityDomains)

		SUM = SUM + sum
		NUM = NUM + num

	}
	printf("\033[1;36m[%s] 结束创建。创建实例总数: %d, 成功 %d , 失败 %d\033[0m\n", oracleSectionName, SUM, NUM, SUM-NUM)
	text := fmt.Sprintf("结束创建。创建实例总数: %d, 成功 %d , 失败 %d", SUM, NUM, SUM-NUM)
	sendMessage(fmt.Sprintf("[%s]", oracleSectionName), text)
}

func multiBatchListInstancesIp() {
	IPsFilePath := IPsFilePrefix + "-" + time.Now().Format("2006-01-02-150405.txt")
	_, err := os.Stat(IPsFilePath)
	if err != nil && os.IsNotExist(err) {
		os.Create(IPsFilePath)
	}

	fmt.Printf("正在导出实例公共IP地址...\n")
	for _, sec := range oracleSections {
		err := initVar(sec)
		if err != nil {
			continue
		}
		ListInstancesIPs(IPsFilePath, sec.Name())
	}
	fmt.Printf("导出实例公共IP地址完成，请查看文件 %s\n", IPsFilePath)
}

func batchListInstancesIp(sec *ini.Section) {
	IPsFilePath := IPsFilePrefix + "-" + time.Now().Format("2006-01-02-150405.txt")
	_, err := os.Stat(IPsFilePath)
	if err != nil && os.IsNotExist(err) {
		os.Create(IPsFilePath)
	}
	fmt.Printf("正在导出实例公共IP地址...\n")
	ListInstancesIPs(IPsFilePath, sec.Name())
	fmt.Printf("导出实例IP地址完成，请查看文件 %s\n", IPsFilePath)
}

func ListInstancesIPs(filePath string, sectionName string) {
	vnicAttachments, err := ListVnicAttachments(ctx, computeClient, nil)
	if err != nil {
		fmt.Printf("ListVnicAttachments Error: %s\n", err.Error())
		return
	}
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, os.ModeAppend)
	if err != nil {
		fmt.Printf("打开文件失败, Error: %s\n", err.Error())
		return
	}
	_, err = io.WriteString(file, "["+sectionName+"]\n")
	if err != nil {
		fmt.Printf("%s\n", err.Error())
	}
	for _, vnicAttachment := range vnicAttachments {
		vnic, err := GetVnic(ctx, networkClient, vnicAttachment.VnicId)
		if err != nil {
			fmt.Printf("IP地址获取失败, %s\n", err.Error())
			continue
		}
		fmt.Printf("[%s] 实例: %s, IP: %s\n", sectionName, *vnic.DisplayName, *vnic.PublicIp)
		_, err = io.WriteString(file, "实例: "+*vnic.DisplayName+", IP: "+*vnic.PublicIp+"\n")
		if err != nil {
			fmt.Printf("写入文件失败, Error: %s\n", err.Error())
		}
	}
	_, err = io.WriteString(file, "\n")
	if err != nil {
		fmt.Printf("%s\n", err.Error())
	}
}

// 返回值 sum: 创建实例总数; num: 创建成功的个数
func LaunchInstances(ads []identity.AvailabilityDomain) (sum, num int32) {
	/* 创建实例的几种情况
	 * 1. 设置了 availabilityDomain 参数，即在设置的可用性域中创建 sum 个实例。
	 * 2. 没有设置 availabilityDomain 但是设置了 each 参数。即在获取的每个可用性域中创建 each 个实例，创建的实例总数 sum =  each * adCount。
	 * 3. 没有设置 availabilityDomain 且没有设置 each 参数，即在获取到的可用性域中创建的实例总数为 sum。
	 */

	//可用性域数量
	var adCount int32 = int32(len(ads))
	adName := common.String(instance.AvailabilityDomain)
	each := instance.Each
	sum = instance.Sum

	// 没有设置可用性域并且没有设置each时，才有用。
	var usableAds = make([]identity.AvailabilityDomain, 0)

	//可用性域不固定，即没有提供 availabilityDomain 参数
	var AD_NOT_FIXED bool = false
	var EACH_AD = false
	if adName == nil || *adName == "" {
		AD_NOT_FIXED = true
		if each > 0 {
			EACH_AD = true
			sum = each * adCount
		} else {
			EACH_AD = false
			usableAds = ads
		}
	}

	name := instance.InstanceDisplayName
	if name == "" {
		name = time.Now().Format("instance-20060102-1504")
	}
	displayName := common.String(name)
	if sum > 1 {
		displayName = common.String(name + "-1")
	}
	// create the launch instance request
	request := core.LaunchInstanceRequest{}
	request.CompartmentId = common.String(oracle.Tenancy)
	request.DisplayName = displayName

	// Get a image.
	fmt.Println("正在获取系统镜像...")
	image, err := GetImage(ctx, computeClient)
	if err != nil {
		printlnErr("获取系统镜像失败", err.Error())
		return
	}
	fmt.Println("系统镜像:", *image.DisplayName)

	var shape core.Shape
	if strings.Contains(strings.ToLower(instance.Shape), "flex") && instance.Ocpus > 0 && instance.MemoryInGBs > 0 {
		shape.Shape = &instance.Shape
		shape.Ocpus = &instance.Ocpus
		shape.MemoryInGBs = &instance.MemoryInGBs
	} else {
		fmt.Println("正在获取Shape信息...")
		shape, err = getShape(image.Id, instance.Shape)
		if err != nil {
			printlnErr("获取Shape信息失败", err.Error())
			return
		}
	}

	request.Shape = shape.Shape
	if strings.Contains(strings.ToLower(*shape.Shape), "flex") {
		request.ShapeConfig = &core.LaunchInstanceShapeConfigDetails{
			Ocpus:       shape.Ocpus,
			MemoryInGBs: shape.MemoryInGBs,
		}
	}

	// create a subnet or get the one already created
	fmt.Println("正在获取子网...")
	subnet, err := CreateOrGetNetworkInfrastructure(ctx, networkClient)
	if err != nil {
		printlnErr("获取子网失败", err.Error())
		return
	}
	fmt.Println("子网:", *subnet.DisplayName)
	request.CreateVnicDetails = &core.CreateVnicDetails{SubnetId: subnet.Id}

	sd := core.InstanceSourceViaImageDetails{}
	sd.ImageId = image.Id
	if instance.BootVolumeSizeInGBs > 0 {
		sd.BootVolumeSizeInGBs = common.Int64(instance.BootVolumeSizeInGBs)
	}
	request.SourceDetails = sd
	request.IsPvEncryptionInTransitEnabled = common.Bool(true)

	metaData := map[string]string{}
	metaData["ssh_authorized_keys"] = instance.SSH_Public_Key
	if instance.CloudInit != "" {
		metaData["user_data"] = instance.CloudInit
	}
	request.Metadata = metaData

	minTime := instance.MinTime
	maxTime := instance.MaxTime

	SKIP_RETRY_MAP := make(map[int32]bool)
	var usableAdsTemp = make([]identity.AvailabilityDomain, 0)

	retry := instance.Retry // 重试次数
	var failTimes int32 = 0 // 失败次数

	// 记录尝试创建实例的次数
	var runTimes int32 = 0

	var adIndex int32 = 0 // 当前可用性域下标
	var pos int32 = 0     // for 循环次数
	var SUCCESS = false   // 创建是否成功

	var startTime = time.Now()

	var bootVolumeSize float64
	if instance.BootVolumeSizeInGBs > 0 {
		bootVolumeSize = float64(instance.BootVolumeSizeInGBs)
	} else {
		bootVolumeSize = math.Round(float64(*image.SizeInMBs) / float64(1024))
	}
	printf("\033[1;36m[%s] 开始创建 %s 实例, OCPU: %g 内存: %g 引导卷: %g \033[0m\n", oracleSectionName, *shape.Shape, *shape.Ocpus, *shape.MemoryInGBs, bootVolumeSize)
	if EACH {
		text := fmt.Sprintf("正在尝试创建第 %d 个实例...⏳\n区域: %s\n实例配置: %s\nOCPU计数: %g\n内存(GB): %g\n引导卷(GB): %g\n创建个数: %d", pos+1, oracle.Region, *shape.Shape, *shape.Ocpus, *shape.MemoryInGBs, bootVolumeSize, sum)
		_, err := sendMessage("", text)
		if err != nil {
			printlnErr("Telegram 消息提醒发送失败", err.Error())
		}
	}

	for pos < sum {

		if AD_NOT_FIXED {
			if EACH_AD {
				if pos%each == 0 && failTimes == 0 {
					adName = ads[adIndex].Name
					adIndex++
				}
			} else {
				if SUCCESS {
					adIndex = 0
				}
				if adIndex >= adCount {
					adIndex = 0
				}
				//adName = ads[adIndex].Name
				adName = usableAds[adIndex].Name
				adIndex++
			}
		}

		runTimes++
		printf("\033[1;36m[%s] 正在尝试创建第 %d 个实例, AD: %s\033[0m\n", oracleSectionName, pos+1, *adName)
		printf("\033[1;36m[%s] 当前尝试次数: %d \033[0m\n", oracleSectionName, runTimes)
		request.AvailabilityDomain = adName
		createResp, err := computeClient.LaunchInstance(ctx, request)

		if err == nil {
			// 创建实例成功
			SUCCESS = true
			num++ //成功个数+1

			duration := fmtDuration(time.Since(startTime))

			printf("\033[1;32m[%s] 第 %d 个实例抢到了🎉, 正在启动中请稍等...⌛️ \033[0m\n", oracleSectionName, pos+1)
			var msg Message
			var msgErr error
			var text string
			if EACH {
				text = fmt.Sprintf("第 %d 个实例抢到了🎉, 正在启动中请稍等...⌛️\n区域: %s\n实例名称: %s\n公共IP: 获取中...⏳\n可用性域:%s\n实例配置: %s\nOCPU计数: %g\n内存(GB): %g\n引导卷(GB): %g\n创建个数: %d\n尝试次数: %d\n耗时: %s", pos+1, oracle.Region, *createResp.Instance.DisplayName, *createResp.Instance.AvailabilityDomain, *shape.Shape, *shape.Ocpus, *shape.MemoryInGBs, bootVolumeSize, sum, runTimes, duration)
				msg, msgErr = sendMessage("", text)
			}
			// 获取实例公共IP
			var strIps string
			ips, err := getInstancePublicIps(createResp.Instance.Id)
			if err != nil {
				printf("\033[1;32m[%s] 第 %d 个实例抢到了🎉, 但是启动失败❌ 错误信息: \033[0m%s\n", oracleSectionName, pos+1, err.Error())
				text = fmt.Sprintf("第 %d 个实例抢到了🎉, 但是启动失败❌实例已被终止😔\n区域: %s\n实例名称: %s\n可用性域:%s\n实例配置: %s\nOCPU计数: %g\n内存(GB): %g\n引导卷(GB): %g\n创建个数: %d\n尝试次数: %d\n耗时: %s", pos+1, oracle.Region, *createResp.Instance.DisplayName, *createResp.Instance.AvailabilityDomain, *shape.Shape, *shape.Ocpus, *shape.MemoryInGBs, bootVolumeSize, sum, runTimes, duration)
			} else {
				strIps = strings.Join(ips, ",")
				printf("\033[1;32m[%s] 第 %d 个实例抢到了🎉, 启动成功✅. 实例名称: %s, 公共IP: %s\033[0m\n", oracleSectionName, pos+1, *createResp.Instance.DisplayName, strIps)
				text = fmt.Sprintf("第 %d 个实例抢到了🎉, 启动成功✅\n区域: %s\n实例名称: %s\n公共IP: %s\n可用性域:%s\n实例配置: %s\nOCPU计数: %g\n内存(GB): %g\n引导卷(GB): %g\n创建个数: %d\n尝试次数: %d\n耗时: %s", pos+1, oracle.Region, *createResp.Instance.DisplayName, strIps, *createResp.Instance.AvailabilityDomain, *shape.Shape, *shape.Ocpus, *shape.MemoryInGBs, bootVolumeSize, sum, runTimes, duration)
			}
			if EACH {
				if msgErr != nil {
					sendMessage("", text)
				} else {
					editMessage(msg.MessageId, "", text)
				}
			}

			sleepRandomSecond(minTime, maxTime)

			displayName = common.String(fmt.Sprintf("%s-%d", name, pos+1))
			request.DisplayName = displayName

		} else {
			// 创建实例失败
			SUCCESS = false
			// 错误信息
			errInfo := err.Error()
			// 是否跳过重试
			SKIP_RETRY := false

			//isRetryable := common.IsErrorRetryableByDefault(err)
			//isNetErr := common.IsNetworkError(err)
			servErr, isServErr := common.IsServiceError(err)

			// API Errors: https://docs.cloud.oracle.com/Content/API/References/apierrors.htm

			if isServErr && (400 <= servErr.GetHTTPStatusCode() && servErr.GetHTTPStatusCode() <= 405) ||
				(servErr.GetHTTPStatusCode() == 409 && !strings.EqualFold(servErr.GetCode(), "IncorrectState")) ||
				servErr.GetHTTPStatusCode() == 412 || servErr.GetHTTPStatusCode() == 413 || servErr.GetHTTPStatusCode() == 422 ||
				servErr.GetHTTPStatusCode() == 431 || servErr.GetHTTPStatusCode() == 501 {
				// 不可重试
				if isServErr {
					errInfo = servErr.GetMessage()
				}
				duration := fmtDuration(time.Since(startTime))
				printf("\033[1;31m[%s] 第 %d 个实例创建失败了❌, 错误信息: \033[0m%s\n", oracleSectionName, pos+1, errInfo)
				if EACH {
					text := fmt.Sprintf("第 %d 个实例创建失败了❌\n错误信息: %s\n区域: %s\n可用性域: %s\n实例配置: %s\nOCPU计数: %g\n内存(GB): %g\n引导卷(GB): %g\n创建个数: %d\n尝试次数: %d\n耗时:%s", pos+1, errInfo, oracle.Region, *adName, *shape.Shape, *shape.Ocpus, *shape.MemoryInGBs, bootVolumeSize, sum, runTimes, duration)
					sendMessage("", text)
				}

				SKIP_RETRY = true
				if AD_NOT_FIXED && !EACH_AD {
					SKIP_RETRY_MAP[adIndex-1] = true
				}

			} else {
				// 可重试
				if isServErr {
					errInfo = servErr.GetMessage()
				}
				printf("\033[1;31m[%s] 创建失败, Error: \033[0m%s\n", oracleSectionName, errInfo)

				SKIP_RETRY = false
				if AD_NOT_FIXED && !EACH_AD {
					SKIP_RETRY_MAP[adIndex-1] = false
				}
			}

			sleepRandomSecond(minTime, maxTime)

			if AD_NOT_FIXED {
				if !EACH_AD {
					if adIndex < adCount {
						// 没有设置可用性域，且没有设置each。即在获取到的每个可用性域里尝试创建。当前使用的可用性域不是最后一个，继续尝试。
						continue
					} else {
						// 当前使用的可用性域是最后一个，判断失败次数是否达到重试次数，未达到重试次数继续尝试。
						failTimes++

						for index, skip := range SKIP_RETRY_MAP {
							if !skip {
								usableAdsTemp = append(usableAdsTemp, usableAds[index])
							}
						}

						// 重新设置 usableAds
						usableAds = usableAdsTemp
						adCount = int32(len(usableAds))

						// 重置变量
						usableAdsTemp = nil
						for k := range SKIP_RETRY_MAP {
							delete(SKIP_RETRY_MAP, k)
						}

						// 判断是否需要重试
						if (retry < 0 || failTimes <= retry) && adCount > 0 {
							continue
						}
					}

					adIndex = 0

				} else {
					// 没有设置可用性域，且设置了each，即在每个域创建each个实例。判断失败次数继续尝试。
					failTimes++
					if (retry < 0 || failTimes <= retry) && !SKIP_RETRY {
						continue
					}
				}

			} else {
				//设置了可用性域，判断是否需要重试
				failTimes++
				if (retry < 0 || failTimes <= retry) && !SKIP_RETRY {
					continue
				}
			}

		}

		// 重置变量
		usableAds = ads
		adCount = int32(len(usableAds))
		usableAdsTemp = nil
		for k := range SKIP_RETRY_MAP {
			delete(SKIP_RETRY_MAP, k)
		}

		// 成功或者失败次数达到重试次数，重置失败次数为0
		failTimes = 0

		// 重置尝试创建实例次数
		runTimes = 0
		startTime = time.Now()

		// for 循环次数+1
		pos++

		if pos < sum && EACH {
			text := fmt.Sprintf("正在尝试创建第 %d 个实例...⏳\n区域: %s\n实例配置: %s\nOCPU计数: %g\n内存(GB): %g\n引导卷(GB): %g\n创建个数: %d", pos+1, oracle.Region, *shape.Shape, *shape.Ocpus, *shape.MemoryInGBs, bootVolumeSize, sum)
			sendMessage("", text)
		}
	}
	return
}

func sleepRandomSecond(min, max int32) {
	var second int32
	if min <= 0 || max <= 0 {
		second = 1
	} else if min >= max {
		second = max
	} else {
		second = rand.Int31n(max-min) + min
	}
	printf("Sleep %d Second...\n", second)
	time.Sleep(time.Duration(second) * time.Second)
}

// ExampleLaunchInstance does create an instance
// NOTE: launch instance will create a new instance and VCN. please make sure delete the instance
// after execute this sample code, otherwise, you will be charged for the running instance
func ExampleLaunchInstance() {
	c, err := core.NewComputeClientWithConfigurationProvider(provider)
	helpers.FatalIfError(err)
	networkClient, err := core.NewVirtualNetworkClientWithConfigurationProvider(provider)
	helpers.FatalIfError(err)
	ctx := context.Background()

	// create the launch instance request
	request := core.LaunchInstanceRequest{}
	request.CompartmentId = common.String(oracle.Tenancy)
	request.DisplayName = common.String(instance.InstanceDisplayName)
	request.AvailabilityDomain = common.String(instance.AvailabilityDomain)

	// create a subnet or get the one already created
	subnet, err := CreateOrGetNetworkInfrastructure(ctx, networkClient)
	helpers.FatalIfError(err)
	fmt.Println("subnet created")
	request.CreateVnicDetails = &core.CreateVnicDetails{SubnetId: subnet.Id}

	// get a image
	images, err := listImages(ctx, c)
	helpers.FatalIfError(err)
	image := images[0]
	fmt.Println("list images")
	request.SourceDetails = core.InstanceSourceViaImageDetails{
		ImageId:             image.Id,
		BootVolumeSizeInGBs: common.Int64(instance.BootVolumeSizeInGBs),
	}

	// use [config.Shape] to create instance
	request.Shape = common.String(instance.Shape)

	request.ShapeConfig = &core.LaunchInstanceShapeConfigDetails{
		Ocpus:       common.Float32(instance.Ocpus),
		MemoryInGBs: common.Float32(instance.MemoryInGBs),
	}

	// add ssh_authorized_keys
	//metaData := map[string]string{
	//	"ssh_authorized_keys": config.SSH_Public_Key,
	//}
	//request.Metadata = metaData
	request.Metadata = map[string]string{"ssh_authorized_keys": instance.SSH_Public_Key}

	// default retry policy will retry on non-200 response
	request.RequestMetadata = helpers.GetRequestMetadataWithDefaultRetryPolicy()

	createResp, err := c.LaunchInstance(ctx, request)
	helpers.FatalIfError(err)

	fmt.Println("launching instance")

	// should retry condition check which returns a bool value indicating whether to do retry or not
	// it checks the lifecycle status equals to Running or not for this case
	shouldRetryFunc := func(r common.OCIOperationResponse) bool {
		if converted, ok := r.Response.(core.GetInstanceResponse); ok {
			return converted.LifecycleState != core.InstanceLifecycleStateRunning
		}
		return true
	}

	// create get instance request with a retry policy which takes a function
	// to determine shouldRetry or not
	pollingGetRequest := core.GetInstanceRequest{
		InstanceId:      createResp.Instance.Id,
		RequestMetadata: helpers.GetRequestMetadataWithCustomizedRetryPolicy(shouldRetryFunc),
	}

	instance, pollError := c.GetInstance(ctx, pollingGetRequest)
	helpers.FatalIfError(pollError)

	fmt.Println("instance launched")

	// 创建辅助 VNIC 并将其附加到指定的实例
	attachVnicResponse, err := c.AttachVnic(context.Background(), core.AttachVnicRequest{
		AttachVnicDetails: core.AttachVnicDetails{
			CreateVnicDetails: &core.CreateVnicDetails{
				SubnetId:       subnet.Id,
				AssignPublicIp: common.Bool(true),
			},
			InstanceId: instance.Id,
		},
	})

	helpers.FatalIfError(err)
	fmt.Println("vnic attached")

	vnicState := attachVnicResponse.VnicAttachment.LifecycleState
	for vnicState != core.VnicAttachmentLifecycleStateAttached {
		time.Sleep(15 * time.Second)
		getVnicAttachmentRequest, err := c.GetVnicAttachment(context.Background(), core.GetVnicAttachmentRequest{
			VnicAttachmentId: attachVnicResponse.Id,
		})
		helpers.FatalIfError(err)
		vnicState = getVnicAttachmentRequest.VnicAttachment.LifecycleState
	}

	// 分离并删除指定的辅助 VNIC
	_, err = c.DetachVnic(context.Background(), core.DetachVnicRequest{
		VnicAttachmentId: attachVnicResponse.Id,
	})

	helpers.FatalIfError(err)
	fmt.Println("vnic dettached")

	defer func() {
		terminateInstance(createResp.Id)

		client, clerr := core.NewVirtualNetworkClientWithConfigurationProvider(common.DefaultConfigProvider())
		helpers.FatalIfError(clerr)

		vcnID := subnet.VcnId
		deleteSubnet(ctx, client, subnet.Id)
		deleteVcn(ctx, client, vcnID)
	}()

	// Output:
	// subnet created
	// list images
	// list shapes
	// launching instance
	// instance launched
	// vnic attached
	// vnic dettached
	// terminating instance
	// instance terminated
	// deleteing subnet
	// subnet deleted
	// deleteing VCN
	// VCN deleted
}

func getProvider(oracle Oracle) (common.ConfigurationProvider, error) {
	content, err := ioutil.ReadFile(oracle.Key_file)
	if err != nil {
		return nil, err
	}
	privateKey := string(content)
	privateKeyPassphrase := common.String(oracle.Key_password)
	return common.NewRawConfigurationProvider(oracle.Tenancy, oracle.User, oracle.Region, oracle.Fingerprint, privateKey, privateKeyPassphrase), nil
}

// 创建或获取基础网络设施
func CreateOrGetNetworkInfrastructure(ctx context.Context, c core.VirtualNetworkClient) (subnet core.Subnet, err error) {
	var vcn core.Vcn
	vcn, err = createOrGetVcn(ctx, c)
	if err != nil {
		return
	}
	var gateway core.InternetGateway
	gateway, err = createOrGetInternetGateway(c, vcn.Id)
	if err != nil {
		return
	}
	_, err = createOrGetRouteTable(c, gateway.Id, vcn.Id)
	if err != nil {
		return
	}
	subnet, err = createOrGetSubnetWithDetails(
		ctx, c, vcn.Id,
		common.String(instance.SubnetDisplayName),
		common.String("10.0.0.0/24"),
		common.String("subnetdns"),
		common.String(instance.AvailabilityDomain))
	return
}

// CreateOrGetSubnetWithDetails either creates a new Virtual Cloud Network (VCN) or get the one already exist
// with detail info
func createOrGetSubnetWithDetails(ctx context.Context, c core.VirtualNetworkClient, vcnID *string,
	displayName *string, cidrBlock *string, dnsLabel *string, availableDomain *string) (subnet core.Subnet, err error) {
	var subnets []core.Subnet
	subnets, err = listSubnets(ctx, c, vcnID)
	if err != nil {
		return
	}

	if displayName == nil {
		displayName = common.String(instance.SubnetDisplayName)
	}

	if len(subnets) > 0 && *displayName == "" {
		subnet = subnets[0]
		return
	}

	// check if the subnet has already been created
	for _, element := range subnets {
		if *element.DisplayName == *displayName {
			// find the subnet, return it
			subnet = element
			return
		}
	}

	// create a new subnet
	fmt.Printf("开始创建Subnet（没有可用的Subnet，或指定的Subnet不存在）\n")
	// 子网名称为空，以当前时间为名称创建子网
	if *displayName == "" {
		displayName = common.String(time.Now().Format("subnet-20060102-1504"))
	}
	request := core.CreateSubnetRequest{}
	//request.AvailabilityDomain = availableDomain //省略此属性创建区域性子网(regional subnet)，提供此属性创建特定于可用性域的子网。建议创建区域性子网。
	request.CompartmentId = &oracle.Tenancy
	request.CidrBlock = cidrBlock
	request.DisplayName = displayName
	request.DnsLabel = dnsLabel
	request.RequestMetadata = getCustomRequestMetadataWithRetryPolicy()

	request.VcnId = vcnID
	var r core.CreateSubnetResponse
	r, err = c.CreateSubnet(ctx, request)
	if err != nil {
		return
	}
	// retry condition check, stop unitl return true
	pollUntilAvailable := func(r common.OCIOperationResponse) bool {
		if converted, ok := r.Response.(core.GetSubnetResponse); ok {
			return converted.LifecycleState != core.SubnetLifecycleStateAvailable
		}
		return true
	}

	pollGetRequest := core.GetSubnetRequest{
		SubnetId:        r.Id,
		RequestMetadata: helpers.GetRequestMetadataWithCustomizedRetryPolicy(pollUntilAvailable),
	}

	// wait for lifecyle become running
	_, err = c.GetSubnet(ctx, pollGetRequest)
	if err != nil {
		return
	}

	// update the security rules
	getReq := core.GetSecurityListRequest{
		SecurityListId:  common.String(r.SecurityListIds[0]),
		RequestMetadata: getCustomRequestMetadataWithRetryPolicy(),
	}

	var getResp core.GetSecurityListResponse
	getResp, err = c.GetSecurityList(ctx, getReq)
	if err != nil {
		return
	}

	// this security rule allows remote control the instance
	/*portRange := core.PortRange{
		Max: common.Int(1521),
		Min: common.Int(1521),
	}*/

	newRules := append(getResp.IngressSecurityRules, core.IngressSecurityRule{
		//Protocol: common.String("6"), // TCP
		Protocol: common.String("all"), // 允许所有协议
		Source:   common.String("0.0.0.0/0"),
		/*TcpOptions: &core.TcpOptions{
			DestinationPortRange: &portRange, // 省略该参数，允许所有目标端口。
		},*/
	})

	updateReq := core.UpdateSecurityListRequest{
		SecurityListId:  common.String(r.SecurityListIds[0]),
		RequestMetadata: getCustomRequestMetadataWithRetryPolicy(),
	}

	updateReq.IngressSecurityRules = newRules

	_, err = c.UpdateSecurityList(ctx, updateReq)
	if err != nil {
		return
	}
	fmt.Printf("Subnet创建成功: %s\n", *r.Subnet.DisplayName)
	subnet = r.Subnet
	return
}

// 列出指定虚拟云网络 (VCN) 中的所有子网
func listSubnets(ctx context.Context, c core.VirtualNetworkClient, vcnID *string) (subnets []core.Subnet, err error) {
	request := core.ListSubnetsRequest{
		CompartmentId:   &oracle.Tenancy,
		VcnId:           vcnID,
		RequestMetadata: getCustomRequestMetadataWithRetryPolicy(),
	}
	var r core.ListSubnetsResponse
	r, err = c.ListSubnets(ctx, request)
	if err != nil {
		return
	}
	subnets = r.Items
	return
}

// 创建一个新的虚拟云网络 (VCN) 或获取已经存在的虚拟云网络
func createOrGetVcn(ctx context.Context, c core.VirtualNetworkClient) (core.Vcn, error) {
	var vcn core.Vcn
	vcnItems, err := listVcns(ctx, c)
	if err != nil {
		return vcn, err
	}
	displayName := common.String(instance.VcnDisplayName)
	if len(vcnItems) > 0 && *displayName == "" {
		vcn = vcnItems[0]
		return vcn, err
	}
	for _, element := range vcnItems {
		if *element.DisplayName == instance.VcnDisplayName {
			// VCN already created, return it
			vcn = element
			return vcn, err
		}
	}
	// create a new VCN
	fmt.Println("开始创建VCN（没有可用的VCN，或指定的VCN不存在）")
	if *displayName == "" {
		displayName = common.String(time.Now().Format("vcn-20060102-1504"))
	}
	request := core.CreateVcnRequest{}
	request.RequestMetadata = getCustomRequestMetadataWithRetryPolicy()
	request.CidrBlock = common.String("10.0.0.0/16")
	request.CompartmentId = common.String(oracle.Tenancy)
	request.DisplayName = displayName
	request.DnsLabel = common.String("vcndns")
	r, err := c.CreateVcn(ctx, request)
	if err != nil {
		return vcn, err
	}
	fmt.Printf("VCN创建成功: %s\n", *r.Vcn.DisplayName)
	vcn = r.Vcn
	return vcn, err
}

// 列出所有虚拟云网络 (VCN)
func listVcns(ctx context.Context, c core.VirtualNetworkClient) ([]core.Vcn, error) {
	request := core.ListVcnsRequest{
		CompartmentId:   &oracle.Tenancy,
		RequestMetadata: getCustomRequestMetadataWithRetryPolicy(),
	}
	r, err := c.ListVcns(ctx, request)
	if err != nil {
		return nil, err
	}
	return r.Items, err
}

// 创建或者获取 Internet 网关
func createOrGetInternetGateway(c core.VirtualNetworkClient, vcnID *string) (core.InternetGateway, error) {
	//List Gateways
	var gateway core.InternetGateway
	listGWRequest := core.ListInternetGatewaysRequest{
		CompartmentId:   &oracle.Tenancy,
		VcnId:           vcnID,
		RequestMetadata: getCustomRequestMetadataWithRetryPolicy(),
	}

	listGWRespone, err := c.ListInternetGateways(ctx, listGWRequest)
	if err != nil {
		fmt.Printf("Internet gateway list error: %s\n", err.Error())
		return gateway, err
	}

	if len(listGWRespone.Items) >= 1 {
		//Gateway with name already exists
		gateway = listGWRespone.Items[0]
	} else {
		//Create new Gateway
		fmt.Printf("开始创建Internet网关\n")
		enabled := true
		createGWDetails := core.CreateInternetGatewayDetails{
			CompartmentId: &oracle.Tenancy,
			IsEnabled:     &enabled,
			VcnId:         vcnID,
		}

		createGWRequest := core.CreateInternetGatewayRequest{
			CreateInternetGatewayDetails: createGWDetails,
			RequestMetadata:              getCustomRequestMetadataWithRetryPolicy()}

		createGWResponse, err := c.CreateInternetGateway(ctx, createGWRequest)

		if err != nil {
			fmt.Printf("Internet gateway create error: %s\n", err.Error())
			return gateway, err
		}
		gateway = createGWResponse.InternetGateway
		fmt.Printf("Internet网关创建成功: %s\n", *gateway.DisplayName)
	}
	return gateway, err
}

// 创建或者获取路由表
func createOrGetRouteTable(c core.VirtualNetworkClient, gatewayID, VcnID *string) (routeTable core.RouteTable, err error) {
	//List Route Table
	listRTRequest := core.ListRouteTablesRequest{
		CompartmentId:   &oracle.Tenancy,
		VcnId:           VcnID,
		RequestMetadata: getCustomRequestMetadataWithRetryPolicy(),
	}
	var listRTResponse core.ListRouteTablesResponse
	listRTResponse, err = c.ListRouteTables(ctx, listRTRequest)
	if err != nil {
		fmt.Printf("Route table list error: %s\n", err.Error())
		return
	}

	cidrRange := "0.0.0.0/0"
	rr := core.RouteRule{
		NetworkEntityId: gatewayID,
		Destination:     &cidrRange,
		DestinationType: core.RouteRuleDestinationTypeCidrBlock,
	}

	if len(listRTResponse.Items) >= 1 {
		//Default Route Table found and has at least 1 route rule
		if len(listRTResponse.Items[0].RouteRules) >= 1 {
			routeTable = listRTResponse.Items[0]
			//Default Route table needs route rule adding
		} else {
			fmt.Printf("路由表未添加规则，开始添加Internet路由规则\n")
			updateRTDetails := core.UpdateRouteTableDetails{
				RouteRules: []core.RouteRule{rr},
			}

			updateRTRequest := core.UpdateRouteTableRequest{
				RtId:                    listRTResponse.Items[0].Id,
				UpdateRouteTableDetails: updateRTDetails,
				RequestMetadata:         getCustomRequestMetadataWithRetryPolicy(),
			}
			var updateRTResponse core.UpdateRouteTableResponse
			updateRTResponse, err = c.UpdateRouteTable(ctx, updateRTRequest)
			if err != nil {
				fmt.Printf("Error updating route table: %s\n", err)
				return
			}
			fmt.Printf("Internet路由规则添加成功\n")
			routeTable = updateRTResponse.RouteTable
		}

	} else {
		//No default route table found
		fmt.Printf("Error could not find VCN default route table, VCN OCID: %s Could not find route table.\n", *VcnID)
	}
	return
}

// 获取符合条件系统镜像中的第一个
func GetImage(ctx context.Context, c core.ComputeClient) (image core.Image, err error) {
	var images []core.Image
	images, err = listImages(ctx, c)
	if err != nil {
		return
	}
	if len(images) > 0 {
		image = images[0]
	} else {
		err = fmt.Errorf("未找到[%s %s]的镜像, 或该镜像不支持[%s]", instance.OperatingSystem, instance.OperatingSystemVersion, instance.Shape)
	}
	return
}

// 列出所有符合条件的系统镜像
func listImages(ctx context.Context, c core.ComputeClient) ([]core.Image, error) {
	if instance.OperatingSystem == "" || instance.OperatingSystemVersion == "" {
		return nil, errors.New("操作系统类型和版本不能为空, 请检查配置文件")
	}
	request := core.ListImagesRequest{
		CompartmentId:          common.String(oracle.Tenancy),
		OperatingSystem:        common.String(instance.OperatingSystem),
		OperatingSystemVersion: common.String(instance.OperatingSystemVersion),
		Shape:                  common.String(instance.Shape),
		RequestMetadata:        getCustomRequestMetadataWithRetryPolicy(),
	}
	r, err := c.ListImages(ctx, request)
	return r.Items, err
}

func getShape(imageId *string, shapeName string) (core.Shape, error) {
	var shape core.Shape
	shapes, err := listShapes(ctx, computeClient, imageId)
	if err != nil {
		return shape, err
	}
	for _, s := range shapes {
		if strings.EqualFold(*s.Shape, shapeName) {
			shape = s
			return shape, nil
		}
	}
	err = errors.New("没有符合条件的Shape")
	return shape, err
}

// ListShapes Lists the shapes that can be used to launch an instance within the specified compartment.
func listShapes(ctx context.Context, c core.ComputeClient, imageID *string) ([]core.Shape, error) {
	request := core.ListShapesRequest{
		CompartmentId:   common.String(oracle.Tenancy),
		ImageId:         imageID,
		RequestMetadata: getCustomRequestMetadataWithRetryPolicy(),
	}
	r, err := c.ListShapes(ctx, request)
	if err == nil && (r.Items == nil || len(r.Items) == 0) {
		err = errors.New("没有符合条件的Shape")
	}
	return r.Items, err
}

// 列出符合条件的可用性域
func ListAvailabilityDomains() ([]identity.AvailabilityDomain, error) {
	req := identity.ListAvailabilityDomainsRequest{
		CompartmentId:   common.String(oracle.Tenancy),
		RequestMetadata: getCustomRequestMetadataWithRetryPolicy(),
	}
	resp, err := identityClient.ListAvailabilityDomains(ctx, req)
	return resp.Items, err
}

func ListInstances(ctx context.Context, c core.ComputeClient) ([]core.Instance, error) {
	req := core.ListInstancesRequest{
		CompartmentId:   common.String(oracle.Tenancy),
		RequestMetadata: getCustomRequestMetadataWithRetryPolicy(),
	}
	resp, err := c.ListInstances(ctx, req)
	return resp.Items, err
}

func ListVnicAttachments(ctx context.Context, c core.ComputeClient, instanceId *string) ([]core.VnicAttachment, error) {
	req := core.ListVnicAttachmentsRequest{
		CompartmentId:   common.String(oracle.Tenancy),
		RequestMetadata: getCustomRequestMetadataWithRetryPolicy()}
	if instanceId != nil && *instanceId != "" {
		req.InstanceId = instanceId
	}
	resp, err := c.ListVnicAttachments(ctx, req)
	return resp.Items, err
}

func GetVnic(ctx context.Context, c core.VirtualNetworkClient, vnicID *string) (core.Vnic, error) {
	req := core.GetVnicRequest{
		VnicId:          vnicID,
		RequestMetadata: getCustomRequestMetadataWithRetryPolicy(),
	}
	resp, err := c.GetVnic(ctx, req)
	if err != nil && resp.RawResponse != nil {
		err = errors.New(resp.RawResponse.Status)
	}
	return resp.Vnic, err
}

// 终止实例
// https://docs.oracle.com/en-us/iaas/api/#/en/iaas/20160918/Instance/TerminateInstance
func terminateInstance(id *string) error {
	request := core.TerminateInstanceRequest{
		InstanceId:         id,
		PreserveBootVolume: common.Bool(false),
		RequestMetadata:    getCustomRequestMetadataWithRetryPolicy(),
	}
	_, err := computeClient.TerminateInstance(ctx, request)
	return err

	//fmt.Println("terminating instance")

	/*
		// should retry condition check which returns a bool value indicating whether to do retry or not
		// it checks the lifecycle status equals to Terminated or not for this case
		shouldRetryFunc := func(r common.OCIOperationResponse) bool {
			if converted, ok := r.Response.(core.GetInstanceResponse); ok {
				return converted.LifecycleState != core.InstanceLifecycleStateTerminated
			}
			return true
		}

		pollGetRequest := core.GetInstanceRequest{
			InstanceId:      id,
			RequestMetadata: helpers.GetRequestMetadataWithCustomizedRetryPolicy(shouldRetryFunc),
		}

		_, pollErr := c.GetInstance(ctx, pollGetRequest)
		helpers.FatalIfError(pollErr)
		fmt.Println("instance terminated")
	*/
}

// 删除虚拟云网络
func deleteVcn(ctx context.Context, c core.VirtualNetworkClient, id *string) {
	request := core.DeleteVcnRequest{
		VcnId:           id,
		RequestMetadata: helpers.GetRequestMetadataWithDefaultRetryPolicy(),
	}

	fmt.Println("deleteing VCN")
	_, err := c.DeleteVcn(ctx, request)
	helpers.FatalIfError(err)

	// should retry condition check which returns a bool value indicating whether to do retry or not
	// it checks the lifecycle status equals to Terminated or not for this case
	shouldRetryFunc := func(r common.OCIOperationResponse) bool {
		if serviceError, ok := common.IsServiceError(r.Error); ok && serviceError.GetHTTPStatusCode() == 404 {
			// resource been deleted, stop retry
			return false
		}

		if converted, ok := r.Response.(core.GetVcnResponse); ok {
			return converted.LifecycleState != core.VcnLifecycleStateTerminated
		}
		return true
	}

	pollGetRequest := core.GetVcnRequest{
		VcnId:           id,
		RequestMetadata: helpers.GetRequestMetadataWithCustomizedRetryPolicy(shouldRetryFunc),
	}

	_, pollErr := c.GetVcn(ctx, pollGetRequest)
	if serviceError, ok := common.IsServiceError(pollErr); !ok ||
		(ok && serviceError.GetHTTPStatusCode() != 404) {
		// fail if the error is not service error or
		// if the error is service error and status code not equals to 404
		helpers.FatalIfError(pollErr)
	}
	fmt.Println("VCN deleted")
}

// 删除子网
func deleteSubnet(ctx context.Context, c core.VirtualNetworkClient, id *string) {
	request := core.DeleteSubnetRequest{
		SubnetId:        id,
		RequestMetadata: helpers.GetRequestMetadataWithDefaultRetryPolicy(),
	}

	_, err := c.DeleteSubnet(context.Background(), request)
	helpers.FatalIfError(err)

	fmt.Println("deleteing subnet")

	// should retry condition check which returns a bool value indicating whether to do retry or not
	// it checks the lifecycle status equals to Terminated or not for this case
	shouldRetryFunc := func(r common.OCIOperationResponse) bool {
		if serviceError, ok := common.IsServiceError(r.Error); ok && serviceError.GetHTTPStatusCode() == 404 {
			// resource been deleted
			return false
		}

		if converted, ok := r.Response.(core.GetSubnetResponse); ok {
			return converted.LifecycleState != core.SubnetLifecycleStateTerminated
		}
		return true
	}

	pollGetRequest := core.GetSubnetRequest{
		SubnetId:        id,
		RequestMetadata: helpers.GetRequestMetadataWithCustomizedRetryPolicy(shouldRetryFunc),
	}

	_, pollErr := c.GetSubnet(ctx, pollGetRequest)
	if serviceError, ok := common.IsServiceError(pollErr); !ok ||
		(ok && serviceError.GetHTTPStatusCode() != 404) {
		// fail if the error is not service error or
		// if the error is service error and status code not equals to 404
		helpers.FatalIfError(pollErr)
	}

	fmt.Println("subnet deleted")
}

func getInstance(instanceId *string) (core.Instance, error) {
	req := core.GetInstanceRequest{
		InstanceId:      instanceId,
		RequestMetadata: getCustomRequestMetadataWithRetryPolicy(),
	}
	resp, err := computeClient.GetInstance(ctx, req)
	return resp.Instance, err
}

func instanceAction(instanceId *string, action core.InstanceActionActionEnum) (ins core.Instance, err error) {
	req := core.InstanceActionRequest{
		InstanceId:      instanceId,
		Action:          action,
		RequestMetadata: getCustomRequestMetadataWithRetryPolicy(),
	}
	resp, err := computeClient.InstanceAction(ctx, req)
	ins = resp.Instance
	return
}

func changePublicIp(vnics []core.Vnic) (publicIp core.PublicIp, err error) {
	var vnic core.Vnic
	for _, v := range vnics {
		if *v.IsPrimary {
			vnic = v
		}
	}
	fmt.Println("正在获取私有IP...")
	var privateIps []core.PrivateIp
	privateIps, err = getPrivateIps(vnic.Id)
	if err != nil {
		printlnErr("获取私有IP失败", err.Error())
		return
	}
	var privateIp core.PrivateIp
	for _, p := range privateIps {
		if *p.IsPrimary {
			privateIp = p
		}
	}

	fmt.Println("正在获取公共IP OCID...")
	publicIp, err = getPublicIp(privateIp.Id)
	if err != nil {
		printlnErr("获取公共IP OCID 失败", err.Error())
	}
	fmt.Println("正在删除公共IP...")
	_, err = deletePublicIp(publicIp.Id)
	if err != nil {
		printlnErr("删除公共IP 失败", err.Error())
	}
	time.Sleep(3 * time.Second)
	fmt.Println("正在创建公共IP...")
	publicIp, err = createPublicIp(privateIp.Id)
	return
}

func getInstanceVnics(instanceId *string) (vnics []core.Vnic, err error) {
	vnicAttachments, err := ListVnicAttachments(ctx, computeClient, instanceId)
	if err != nil {
		return
	}
	for _, vnicAttachment := range vnicAttachments {
		vnic, vnicErr := GetVnic(ctx, networkClient, vnicAttachment.VnicId)
		if vnicErr != nil {
			fmt.Printf("GetVnic error: %s\n", vnicErr.Error())
			continue
		}
		vnics = append(vnics, vnic)
	}
	return
}

// 更新指定的VNIC
func updateVnic(vnicId *string) (core.Vnic, error) {
	req := core.UpdateVnicRequest{
		VnicId:            vnicId,
		UpdateVnicDetails: core.UpdateVnicDetails{SkipSourceDestCheck: common.Bool(true)},
		RequestMetadata:   getCustomRequestMetadataWithRetryPolicy(),
	}
	resp, err := networkClient.UpdateVnic(ctx, req)
	return resp.Vnic, err
}

// 获取指定VNIC的私有IP
func getPrivateIps(vnicId *string) ([]core.PrivateIp, error) {
	req := core.ListPrivateIpsRequest{
		VnicId:          vnicId,
		RequestMetadata: getCustomRequestMetadataWithRetryPolicy(),
	}
	resp, err := networkClient.ListPrivateIps(ctx, req)
	if err == nil && (resp.Items == nil || len(resp.Items) == 0) {
		err = errors.New("私有IP为空")
	}
	return resp.Items, err
}

// 获取分配给指定私有IP的公共IP
func getPublicIp(privateIpId *string) (core.PublicIp, error) {
	req := core.GetPublicIpByPrivateIpIdRequest{
		GetPublicIpByPrivateIpIdDetails: core.GetPublicIpByPrivateIpIdDetails{PrivateIpId: privateIpId},
		RequestMetadata:                 getCustomRequestMetadataWithRetryPolicy(),
	}
	resp, err := networkClient.GetPublicIpByPrivateIpId(ctx, req)
	if err == nil && resp.PublicIp.Id == nil {
		err = errors.New("未分配公共IP")
	}
	return resp.PublicIp, err
}

// 删除公共IP
// 取消分配并删除指定公共IP（临时或保留）
// 如果仅需要取消分配保留的公共IP并将保留的公共IP返回到保留公共IP池，请使用updatePublicIp方法。
func deletePublicIp(publicIpId *string) (core.DeletePublicIpResponse, error) {
	req := core.DeletePublicIpRequest{
		PublicIpId:      publicIpId,
		RequestMetadata: getCustomRequestMetadataWithRetryPolicy()}
	return networkClient.DeletePublicIp(ctx, req)
}

// 创建公共IP
// 通过Lifetime指定创建临时公共IP还是保留公共IP。
// 创建临时公共IP，必须指定privateIpId，将临时公共IP分配给指定私有IP。
// 创建保留公共IP，可以不指定privateIpId。稍后可以使用updatePublicIp方法分配给私有IP。
func createPublicIp(privateIpId *string) (core.PublicIp, error) {
	var publicIp core.PublicIp
	req := core.CreatePublicIpRequest{
		CreatePublicIpDetails: core.CreatePublicIpDetails{
			CompartmentId: common.String(oracle.Tenancy),
			Lifetime:      core.CreatePublicIpDetailsLifetimeEphemeral,
			PrivateIpId:   privateIpId,
		},
		RequestMetadata: getCustomRequestMetadataWithRetryPolicy(),
	}
	resp, err := networkClient.CreatePublicIp(ctx, req)
	publicIp = resp.PublicIp
	return publicIp, err
}

// 更新保留公共IP
// 1. 将保留的公共IP分配给指定的私有IP。如果该公共IP已经分配给私有IP，会取消分配，然后重新分配给指定的私有IP。
// 2. PrivateIpId设置为空字符串，公共IP取消分配到关联的私有IP。
func updatePublicIp(publicIpId *string, privateIpId *string) (core.PublicIp, error) {
	req := core.UpdatePublicIpRequest{
		PublicIpId: publicIpId,
		UpdatePublicIpDetails: core.UpdatePublicIpDetails{
			PrivateIpId: privateIpId,
		},
		RequestMetadata: getCustomRequestMetadataWithRetryPolicy(),
	}
	resp, err := networkClient.UpdatePublicIp(ctx, req)
	return resp.PublicIp, err
}

// 根据实例OCID获取公共IP
func getInstancePublicIps(instanceId *string) (ips []string, err error) {
	// 多次尝试，避免刚抢购到实例，实例正在预配获取不到公共IP。
	var ins core.Instance
	for i := 0; i < 100; i++ {
		if ins.LifecycleState != core.InstanceLifecycleStateRunning {
			ins, err = getInstance(instanceId)
			if err != nil {
				continue
			}
			if ins.LifecycleState == core.InstanceLifecycleStateTerminating || ins.LifecycleState == core.InstanceLifecycleStateTerminated {
				err = errors.New("实例已终止😔")
				return
			}
			// if ins.LifecycleState != core.InstanceLifecycleStateRunning {
			// 	continue
			// }
		}

		var vnicAttachments []core.VnicAttachment
		vnicAttachments, err = ListVnicAttachments(ctx, computeClient, instanceId)
		if err != nil {
			continue
		}
		if len(vnicAttachments) > 0 {
			for _, vnicAttachment := range vnicAttachments {
				vnic, vnicErr := GetVnic(ctx, networkClient, vnicAttachment.VnicId)
				if vnicErr != nil {
					printf("GetVnic error: %s\n", vnicErr.Error())
					continue
				}
				if vnic.PublicIp != nil && *vnic.PublicIp != "" {
					ips = append(ips, *vnic.PublicIp)
				}
			}
			return
		}
		time.Sleep(3 * time.Second)
	}
	return
}

// 列出引导卷
func getBootVolumes(availabilityDomain *string) ([]core.BootVolume, error) {
	req := core.ListBootVolumesRequest{
		AvailabilityDomain: availabilityDomain,
		CompartmentId:      common.String(oracle.Tenancy),
		RequestMetadata:    getCustomRequestMetadataWithRetryPolicy(),
	}
	resp, err := storageClient.ListBootVolumes(ctx, req)
	return resp.Items, err
}

// 获取指定引导卷
func getBootVolume(bootVolumeId *string) (core.BootVolume, error) {
	req := core.GetBootVolumeRequest{
		BootVolumeId:    bootVolumeId,
		RequestMetadata: getCustomRequestMetadataWithRetryPolicy(),
	}
	resp, err := storageClient.GetBootVolume(ctx, req)
	return resp.BootVolume, err
}

// 更新引导卷
func updateBootVolume(bootVolumeId *string, sizeInGBs *int64, vpusPerGB *int64) (core.BootVolume, error) {
	updateBootVolumeDetails := core.UpdateBootVolumeDetails{}
	if sizeInGBs != nil {
		updateBootVolumeDetails.SizeInGBs = sizeInGBs
	}
	if vpusPerGB != nil {
		updateBootVolumeDetails.VpusPerGB = vpusPerGB
	}
	req := core.UpdateBootVolumeRequest{
		BootVolumeId:            bootVolumeId,
		UpdateBootVolumeDetails: updateBootVolumeDetails,
		RequestMetadata:         getCustomRequestMetadataWithRetryPolicy(),
	}
	resp, err := storageClient.UpdateBootVolume(ctx, req)
	return resp.BootVolume, err
}

// 删除引导卷
func deleteBootVolume(bootVolumeId *string) (*http.Response, error) {
	req := core.DeleteBootVolumeRequest{
		BootVolumeId:    bootVolumeId,
		RequestMetadata: getCustomRequestMetadataWithRetryPolicy(),
	}
	resp, err := storageClient.DeleteBootVolume(ctx, req)
	return resp.RawResponse, err
}

// 分离引导卷
func detachBootVolume(bootVolumeAttachmentId *string) (*http.Response, error) {
	req := core.DetachBootVolumeRequest{
		BootVolumeAttachmentId: bootVolumeAttachmentId,
		RequestMetadata:        getCustomRequestMetadataWithRetryPolicy(),
	}
	resp, err := computeClient.DetachBootVolume(ctx, req)
	return resp.RawResponse, err
}

// 获取引导卷附件
func listBootVolumeAttachments(availabilityDomain, compartmentId, bootVolumeId *string) ([]core.BootVolumeAttachment, error) {
	req := core.ListBootVolumeAttachmentsRequest{
		AvailabilityDomain: availabilityDomain,
		CompartmentId:      compartmentId,
		BootVolumeId:       bootVolumeId,
		RequestMetadata:    getCustomRequestMetadataWithRetryPolicy(),
	}
	resp, err := computeClient.ListBootVolumeAttachments(ctx, req)
	return resp.Items, err
}

func sendMessage(name, text string) (msg Message, err error) {
	if token != "" && chat_id != "" {
		data := url.Values{
			"parse_mode": {"Markdown"},
			"chat_id":    {chat_id},
			"text":       {"🔰*甲骨文通知* " + name + "\n" + text},
		}
		var req *http.Request
		req, err = http.NewRequest(http.MethodPost, sendMessageUrl, strings.NewReader(data.Encode()))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		client := common.BaseClient{HTTPClient: &http.Client{}}
		setProxyOrNot(&client)
		var resp *http.Response
		resp, err = client.HTTPClient.Do(req)
		if err != nil {
			return
		}
		var body []byte
		body, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			return
		}
		err = json.Unmarshal(body, &msg)
		if err != nil {
			return
		}
		if !msg.OK {
			err = errors.New(msg.Description)
			return
		}
	}
	return
}

func editMessage(messageId int, name, text string) (msg Message, err error) {
	if token != "" && chat_id != "" {
		data := url.Values{
			"parse_mode": {"Markdown"},
			"chat_id":    {chat_id},
			"message_id": {strconv.Itoa(messageId)},
			"text":       {"🔰*甲骨文通知* " + name + "\n" + text},
		}
		var req *http.Request
		req, err = http.NewRequest(http.MethodPost, editMessageUrl, strings.NewReader(data.Encode()))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		client := common.BaseClient{HTTPClient: &http.Client{}}
		setProxyOrNot(&client)
		var resp *http.Response
		resp, err = client.HTTPClient.Do(req)
		if err != nil {
			return
		}
		var body []byte
		body, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			return
		}
		err = json.Unmarshal(body, &msg)
		if err != nil {
			return
		}
		if !msg.OK {
			err = errors.New(msg.Description)
			return
		}

	}
	return
}

func setProxyOrNot(client *common.BaseClient) {
	if proxy != "" {
		proxyURL, err := url.Parse(proxy)
		if err != nil {
			printlnErr("URL parse failed", err.Error())
			return
		}
		client.HTTPClient = &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
			},
		}
	}
}

func getInstanceState(state core.InstanceLifecycleStateEnum) string {
	var friendlyState string
	switch state {
	case core.InstanceLifecycleStateMoving:
		friendlyState = "正在移动"
	case core.InstanceLifecycleStateProvisioning:
		friendlyState = "正在预配"
	case core.InstanceLifecycleStateRunning:
		friendlyState = "正在运行"
	case core.InstanceLifecycleStateStarting:
		friendlyState = "正在启动"
	case core.InstanceLifecycleStateStopping:
		friendlyState = "正在停止"
	case core.InstanceLifecycleStateStopped:
		friendlyState = "已停止　"
	case core.InstanceLifecycleStateTerminating:
		friendlyState = "正在终止"
	case core.InstanceLifecycleStateTerminated:
		friendlyState = "已终止　"
	default:
		friendlyState = string(state)
	}
	return friendlyState
}

func getBootVolumeState(state core.BootVolumeLifecycleStateEnum) string {
	var friendlyState string
	switch state {
	case core.BootVolumeLifecycleStateProvisioning:
		friendlyState = "正在预配"
	case core.BootVolumeLifecycleStateRestoring:
		friendlyState = "正在恢复"
	case core.BootVolumeLifecycleStateAvailable:
		friendlyState = "可用　　"
	case core.BootVolumeLifecycleStateTerminating:
		friendlyState = "正在终止"
	case core.BootVolumeLifecycleStateTerminated:
		friendlyState = "已终止　"
	case core.BootVolumeLifecycleStateFaulty:
		friendlyState = "故障　　"
	default:
		friendlyState = string(state)
	}
	return friendlyState
}

func fmtDuration(d time.Duration) string {
	if d.Seconds() < 1 {
		return "< 1 秒"
	}
	var buffer bytes.Buffer
	//days := int(d.Hours() / 24)
	//hours := int(math.Mod(d.Hours(), 24))
	//minutes := int(math.Mod(d.Minutes(), 60))
	//seconds := int(math.Mod(d.Seconds(), 60))

	days := int(d / (time.Hour * 24))
	hours := int((d % (time.Hour * 24)).Hours())
	minutes := int((d % time.Hour).Minutes())
	seconds := int((d % time.Minute).Seconds())

	if days > 0 {
		buffer.WriteString(fmt.Sprintf("%d 天 ", days))
	}
	if hours > 0 {
		buffer.WriteString(fmt.Sprintf("%d 时 ", hours))
	}
	if minutes > 0 {
		buffer.WriteString(fmt.Sprintf("%d 分 ", minutes))
	}
	if seconds > 0 {
		buffer.WriteString(fmt.Sprintf("%d 秒", seconds))
	}
	return buffer.String()
}

func printf(format string, a ...interface{}) {
	fmt.Printf("%s ", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Printf(format, a...)
}

func printlnErr(desc, detail string) {
	fmt.Printf("\033[1;31mError: %s. %s\033[0m\n", desc, detail)
}

func getCustomRequestMetadataWithRetryPolicy() common.RequestMetadata {
	return common.RequestMetadata{
		RetryPolicy: getCustomRetryPolicy(),
	}
}

func getCustomRetryPolicy() *common.RetryPolicy {
	// how many times to do the retry
	attempts := uint(3)
	// retry for all non-200 status code
	retryOnAllNon200ResponseCodes := func(r common.OCIOperationResponse) bool {
		return !(r.Error == nil && 199 < r.Response.HTTPResponse().StatusCode && r.Response.HTTPResponse().StatusCode < 300)
	}
	policy := common.NewRetryPolicyWithOptions(
		// only base off DefaultRetryPolicyWithoutEventualConsistency() if we're not handling eventual consistency
		common.WithConditionalOption(!false, common.ReplaceWithValuesFromRetryPolicy(common.DefaultRetryPolicyWithoutEventualConsistency())),
		common.WithMaximumNumberAttempts(attempts),
		common.WithShouldRetryOperation(retryOnAllNon200ResponseCodes))
	return &policy
}
