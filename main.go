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
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/oracle/oci-go-sdk/v49/common"
	"github.com/oracle/oci-go-sdk/v49/core"
	"github.com/oracle/oci-go-sdk/v49/example/helpers"
	"github.com/oracle/oci-go-sdk/v49/identity"
	"gopkg.in/ini.v1"
)

const (
	defConfigFilePath = "./oci-help.ini"
	IPsFilePrefix     = "IPs"
)

var (
	provider       common.ConfigurationProvider
	computeClient  core.ComputeClient
	networkClient  core.VirtualNetworkClient
	ctx            context.Context
	configFilePath string
	sections       []*ini.Section
	section        *ini.Section
	config         Config
	providerName   string
	proxy          string
	token          string
	chat_id        string
	sendMessageUrl string
	EACH           bool
)

type Config struct {
	AvailabilityDomain     string  `ini:"availabilityDomain"`
	SSH_Public_Key         string  `ini:"ssh_authorized_key"`
	CompartmentID          string  `ini:"tenancy"`
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
	rand.Seed(time.Now().UnixNano())

	secs := cfg.Sections()
	sections = []*ini.Section{}
	for _, sec := range secs {
		if len(sec.ParentKeys()) == 0 {
			user := sec.Key("user").Value()
			fingerprint := sec.Key("fingerprint").Value()
			tenancy := sec.Key("tenancy").Value()
			region := sec.Key("region").Value()
			key_file := sec.Key("key_file").Value()
			if user != "" && fingerprint != "" && tenancy != "" && region != "" && key_file != "" {
				sections = append(sections, sec)
			}
		}
	}
	if len(sections) == 0 {
		fmt.Printf("\033[1;31m未找到正确的配置信息, 请参考链接文档配置相关信息。链接: https://github.com/lemoex/oci-help\033[0m\n")
		return
	}

	listOracleAccount()
}

func listOracleAccount() {
	if len(sections) == 1 {
		section = sections[0]
	} else {
		fmt.Printf("\n\033[1;32m%s\033[0m\n\n", "欢迎使用甲骨文实例管理工具")
		w := new(tabwriter.Writer)
		w.Init(os.Stdout, 4, 8, 2, '\t', 0)
		fmt.Fprintf(w, "%s\t%s\t%s\t\n", "序号", "账号", "区域")
		for i, section := range sections {
			fmt.Fprintf(w, "%d\t%s\t%s\t\n", i+1, section.Name(), section.Key("region").Value())
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
			if 0 < index && index <= len(sections) {
				break
			} else {
				index = 0
				input = ""
				fmt.Printf("\033[1;31m错误! 请输入正确的序号\033[0m\n")
			}
		}
		section = sections[index-1]
	}
	var err error
	ctx = context.Background()
	provider, err = getProvider(configFilePath, section.Name(), "")
	helpers.FatalIfError(err)
	computeClient, err = core.NewComputeClientWithConfigurationProvider(provider)
	helpers.FatalIfError(err)
	setProxyOrNot(&computeClient.BaseClient)
	networkClient, err = core.NewVirtualNetworkClientWithConfigurationProvider(provider)
	helpers.FatalIfError(err)
	setProxyOrNot(&networkClient.BaseClient)
	showMainMenu()
}

func showMainMenu() {
	fmt.Printf("\n\033[1;32m欢迎使用甲骨文实例管理工具\033[0m \n(当前账号: %s)\n\n", section.Name())
	fmt.Printf("\033[1;36m%s\033[0m %s\n", "1.", "查看实例")
	fmt.Printf("\033[1;36m%s\033[0m %s\n", "2.", "创建实例")
	fmt.Print("\n请输入序号进入相关操作: ")
	var input string
	var num int
	fmt.Scanln(&input)
	if strings.EqualFold(input, "oci") {
		batchLaunchInstances(section, section.ChildSections())
		showMainMenu()
		return
	} else if strings.EqualFold(input, "ip") {
		batchListInstancesIp(section)
		showMainMenu()
		return
	}
	num, _ = strconv.Atoi(input)
	switch num {
	case 1:
		listInstances()
	case 2:
		listLaunchInstanceTemplates()
	default:
		if len(sections) > 1 {
			listOracleAccount()
		}
	}
}

func listLaunchInstanceTemplates() {
	childSections := section.ChildSections()
	if len(childSections) == 0 {
		fmt.Printf("\033[1;31m未找到实例模版, 回车返回上一级菜单.\033[0m")
		fmt.Scanln()
		showMainMenu()
		return
	}

	for {
		fmt.Printf("\n\033[1;32m%s\033[0m\n\n", "选择对应的实例模版开始创建实例")
		w := new(tabwriter.Writer)
		w.Init(os.Stdout, 4, 8, 2, '\t', 0)
		fmt.Fprintf(w, "%s\t%s\t%s\t\n", "序号", "名称", "配置")
		for i, child := range childSections {
			fmt.Fprintf(w, "%d\t%s\t%s\t\n", i+1, child.Name(), child.Key("shape").Value())
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
			if 0 < index && index <= len(childSections) {
				break
			} else {
				input = ""
				index = 0
				fmt.Printf("\033[1;31m错误! 请输入正确的序号\033[0m\n")
			}
		}

		childSection := childSections[index-1]
		// 获取可用性域
		availabilityDomains, err := ListAvailabilityDomains()
		if err != nil {
			fmt.Printf("\033[1;31m获取可用性域失败.\033[0m %s\n", err.Error())
			continue
		}
		providerName = childSection.Name()
		config = Config{}
		err = childSection.MapTo(&config)
		if err != nil {
			fmt.Printf("\033[1;31m解析实例参数失败.\033[0m %s\n", err.Error())
			continue
		}

		LaunchInstances(availabilityDomains)
	}

}

func listInstances() {
	fmt.Println("正在获取实例数据...")
	instances, err := ListInstances(ctx, computeClient)
	if err != nil {
		fmt.Printf("\033[1;31m获取失败, 回车返回上一级菜单.\033[0m")
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
	fmt.Printf("\n\033[1;32m实例信息\033[0m \n(当前账号: %s)\n\n", section.Name())
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
	fmt.Printf("\n")
	var input string
	var index int
	for {
		fmt.Print("请输入序号查看实例详细信息: ")
		_, err := fmt.Scanln(&input)
		if err != nil {
			showMainMenu()
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

		fmt.Printf("\n\033[1;32m实例详细信息\033[0m \n(当前账号: %s)\n\n", section.Name())
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

func getInstance(instanceId *string) (core.Instance, error) {
	req := core.GetInstanceRequest{
		InstanceId: instanceId,
	}
	resp, err := computeClient.GetInstance(ctx, req)
	return resp.Instance, err
}

func instanceAction(instanceId *string, action core.InstanceActionActionEnum) (ins core.Instance, err error) {
	req := core.InstanceActionRequest{
		InstanceId: instanceId,
		Action:     action,
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
	var privateIps []core.PrivateIp
	privateIps, err = getPrivateIps(vnic.Id)
	if err != nil {
		return
	}
	var privateIp core.PrivateIp
	for _, p := range privateIps {
		if *p.IsPrimary {
			privateIp = p
		}
	}

	publicIp, err = getPublicIp(privateIp.Id)
	if err != nil {
		fmt.Println(err.Error())
	}
	_, err = deletePublicIp(publicIp.Id)
	if err != nil {
		fmt.Println(err.Error())
	}
	time.Sleep(3 * time.Second)
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
			printf("GetVnic error: %s\n", vnicErr.Error())
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
	}
	resp, err := networkClient.UpdateVnic(ctx, req)
	return resp.Vnic, err
}

// 获取指定VNIC的私有IP
func getPrivateIps(vnicId *string) ([]core.PrivateIp, error) {
	req := core.ListPrivateIpsRequest{
		VnicId: vnicId,
	}
	resp, err := networkClient.ListPrivateIps(ctx, req)
	return resp.Items, err
}

// 获取分配给指定私有IP的公共IP
func getPublicIp(privateIpId *string) (core.PublicIp, error) {
	req := core.GetPublicIpByPrivateIpIdRequest{
		GetPublicIpByPrivateIpIdDetails: core.GetPublicIpByPrivateIpIdDetails{PrivateIpId: privateIpId},
	}
	resp, err := networkClient.GetPublicIpByPrivateIpId(ctx, req)
	return resp.PublicIp, err
}

// 删除公共IP
// 取消分配并删除指定公共IP（临时或保留）
// 如果仅需要取消分配保留的公共IP并将保留的公共IP返回到保留公共IP池，请使用updatePublicIp方法。
func deletePublicIp(publicIpId *string) (core.DeletePublicIpResponse, error) {
	req := core.DeletePublicIpRequest{PublicIpId: publicIpId}
	return networkClient.DeletePublicIp(ctx, req)
}

// 创建公共IP
// 通过Lifetime指定创建临时公共IP还是保留公共IP。
// 创建临时公共IP，必须指定privateIpId，将临时公共IP分配给指定私有IP。
// 创建保留公共IP，可以不指定privateIpId。稍后可以使用updatePublicIp方法分配给私有IP。
func createPublicIp(privateIpId *string) (core.PublicIp, error) {
	var publicIp core.PublicIp
	var compartmentId string
	compartmentId, err := provider.TenancyOCID()
	if err != nil {
		return publicIp, err
	}
	req := core.CreatePublicIpRequest{
		CreatePublicIpDetails: core.CreatePublicIpDetails{
			CompartmentId: common.String(compartmentId),
			Lifetime:      core.CreatePublicIpDetailsLifetimeEphemeral,
			PrivateIpId:   privateIpId,
		},
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
	}
	resp, err := networkClient.UpdatePublicIp(ctx, req)
	return resp.PublicIp, err
}

func multiBatchLaunchInstances() {
	for _, sec := range sections {
		childSections := sec.ChildSections()
		if len(childSections) > 0 {
			var err error
			ctx = context.Background()
			provider, err = getProvider(configFilePath, sec.Name(), "")
			if err != nil {
				fmt.Println(err)
				return
			}
			computeClient, err = core.NewComputeClientWithConfigurationProvider(provider)
			if err != nil {
				fmt.Println(err)
				return
			}
			setProxyOrNot(&computeClient.BaseClient)
			networkClient, err = core.NewVirtualNetworkClientWithConfigurationProvider(provider)
			if err != nil {
				fmt.Println(err)
				return
			}
			setProxyOrNot(&networkClient.BaseClient)

			batchLaunchInstances(sec, childSections)
		}
	}
}

func batchLaunchInstances(sec *ini.Section, childSections []*ini.Section) {
	if len(childSections) == 0 {
		return
	}
	// 获取可用性域
	AvailabilityDomains, err := ListAvailabilityDomains()

	printf("\033[1;36m[%s] 开始创建\033[0m\n", sec.Name())
	var SUM, NUM int32 = 0, 0
	sendMessage(sec.Name(), "开始创建")

	if err != nil {
		fmt.Println(err)
		return
	}
	for _, child := range childSections {
		providerName = child.Name()
		config = Config{}
		err := child.MapTo(&config)
		if err != nil {
			fmt.Println(err)
			return
		}

		sum, num := LaunchInstances(AvailabilityDomains)

		SUM = SUM + sum
		NUM = NUM + num

	}
	printf("\033[1;36m[%s] 结束创建。创建实例总数: %d, 成功 %d , 失败 %d\033[0m\n", sec.Name(), SUM, NUM, SUM-NUM)
	text := fmt.Sprintf("结束创建。创建实例总数: %d, 成功 %d , 失败 %d", SUM, NUM, SUM-NUM)
	sendMessage(sec.Name(), text)

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
	adName := common.String(config.AvailabilityDomain)
	each := config.Each
	sum = config.Sum

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

	name := config.InstanceDisplayName
	if name == "" {
		name = time.Now().Format("instance-20060102-1504")
	}
	displayName := common.String(name)
	if sum > 1 {
		displayName = common.String(name + "-1")
	}
	// create the launch instance request
	request := core.LaunchInstanceRequest{}
	request.CompartmentId = common.String(config.CompartmentID)
	request.DisplayName = displayName
	// create a subnet or get the one already created
	subnet, err := CreateOrGetNetworkInfrastructure(ctx, networkClient)
	if err != nil {
		fmt.Println(err)
		return
	}
	printf("获取子网: %s\n", *subnet.DisplayName)
	request.CreateVnicDetails = &core.CreateVnicDetails{SubnetId: subnet.Id}
	// Get a image.
	image, err := GetImage(ctx, computeClient)
	if err != nil {
		fmt.Println(err)
		return
	}
	printf("获取系统: %s\n", *image.DisplayName)
	sd := core.InstanceSourceViaImageDetails{}
	sd.ImageId = image.Id
	if config.BootVolumeSizeInGBs > 0 {
		sd.BootVolumeSizeInGBs = common.Int64(config.BootVolumeSizeInGBs)
	}
	request.SourceDetails = sd
	request.IsPvEncryptionInTransitEnabled = common.Bool(true)
	request.Shape = common.String(config.Shape)
	if config.Ocpus > 0 && config.MemoryInGBs > 0 {
		request.ShapeConfig = &core.LaunchInstanceShapeConfigDetails{
			Ocpus:       common.Float32(config.Ocpus),
			MemoryInGBs: common.Float32(config.MemoryInGBs),
		}
	}
	metaData := map[string]string{}
	metaData["ssh_authorized_keys"] = config.SSH_Public_Key
	if config.CloudInit != "" {
		metaData["user_data"] = config.CloudInit
	}
	request.Metadata = metaData

	printf("\033[1;36m[%s] 开始创建...\033[0m\n", providerName)
	if EACH {
		sendMessage(providerName, "开始尝试创建实例...")
	}

	minTime := config.MinTime
	maxTime := config.MaxTime

	SKIP_RETRY_MAP := make(map[int32]bool)
	var usableAdsTemp = make([]identity.AvailabilityDomain, 0)

	retry := config.Retry   // 重试次数
	var failTimes int32 = 0 // 失败次数

	// 记录尝试创建实例的次数
	var runTimes int32 = 0

	var adIndex int32 = 0 // 当前可用性域下标
	var pos int32 = 0     // for 循环次数
	var SUCCESS = false   // 创建是否成功

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
		printf("\033[1;36m[%s] 正在尝试创建第 %d 个实例, AD: %s\033[0m\n", providerName, pos+1, *adName)
		printf("\033[1;36m[%s] 当前尝试次数: %d \033[0m\n", providerName, runTimes)
		request.AvailabilityDomain = adName
		createResp, err := computeClient.LaunchInstance(ctx, request)

		if err == nil {
			// 创建实例成功
			SUCCESS = true
			num++ //成功个数+1

			// 获取实例公共IP
			ips, err := getInstancePublicIps(ctx, computeClient, networkClient, createResp.Instance.Id)
			var strIps string
			if err != nil {
				strIps = err.Error()
			} else {
				strIps = strings.Join(ips, ",")
			}

			printf("\033[1;32m[%s] 第 %d 个实例创建成功. 实例名称: %s, 公网IP: %s\033[0m\n", providerName, pos+1, *createResp.Instance.DisplayName, strIps)
			if EACH {
				sendMessage(providerName, fmt.Sprintf("经过 %d 次尝试, 第%d个实例创建成功🎉\n实例名称: %s\n公网IP: %s", runTimes, pos+1, *createResp.Instance.DisplayName, strIps))
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
				printf("\033[1;31m[%s] 创建失败, Error: \033[0m%s\n", providerName, errInfo)
				if EACH {
					sendMessage(providerName, "创建失败，Error: "+errInfo)
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
				printf("\033[1;31m[%s] 创建失败, Error: \033[0m%s\n", providerName, errInfo)

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

		// for 循环次数+1
		pos++
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

func multiBatchListInstancesIp() {
	IPsFilePath := IPsFilePrefix + "-" + time.Now().Format("2006-01-02-150405.txt")
	_, err := os.Stat(IPsFilePath)
	if err != nil && os.IsNotExist(err) {
		os.Create(IPsFilePath)
	}

	fmt.Printf("正在获取所有实例公共IP地址...\n")
	for _, sec := range sections {
		var err error
		ctx = context.Background()
		provider, err = getProvider(configFilePath, sec.Name(), "")
		if err != nil {
			fmt.Println(err)
			return
		}
		computeClient, err = core.NewComputeClientWithConfigurationProvider(provider)
		if err != nil {
			fmt.Println(err)
			return
		}
		setProxyOrNot(&computeClient.BaseClient)
		networkClient, err = core.NewVirtualNetworkClientWithConfigurationProvider(provider)
		if err != nil {
			fmt.Println(err)
			return
		}
		setProxyOrNot(&networkClient.BaseClient)

		ListInstancesIPs(IPsFilePath, sec.Name())
	}
	fmt.Printf("获取所有实例公共IP地址完成，请查看文件 %s\n", IPsFilePath)
}

func batchListInstancesIp(sec *ini.Section) {
	IPsFilePath := IPsFilePrefix + "-" + time.Now().Format("2006-01-02-150405.txt")
	_, err := os.Stat(IPsFilePath)
	if err != nil && os.IsNotExist(err) {
		os.Create(IPsFilePath)
	}
	fmt.Printf("正在获取所有实例公共IP地址...\n")
	ListInstancesIPs(IPsFilePath, sec.Name())
	fmt.Printf("获取所有实例IP地址完成，请查看文件 %s\n", IPsFilePath)
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
	request.CompartmentId = common.String(config.CompartmentID)
	request.DisplayName = common.String(config.InstanceDisplayName)
	request.AvailabilityDomain = common.String(config.AvailabilityDomain)

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
		BootVolumeSizeInGBs: common.Int64(config.BootVolumeSizeInGBs),
	}

	// use [config.Shape] to create instance
	request.Shape = common.String(config.Shape)

	request.ShapeConfig = &core.LaunchInstanceShapeConfigDetails{
		Ocpus:       common.Float32(config.Ocpus),
		MemoryInGBs: common.Float32(config.MemoryInGBs),
	}

	// add ssh_authorized_keys
	//metaData := map[string]string{
	//	"ssh_authorized_keys": config.SSH_Public_Key,
	//}
	//request.Metadata = metaData
	request.Metadata = map[string]string{"ssh_authorized_keys": config.SSH_Public_Key}

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

func getProvider(configPath, profile, privateKeyPassword string) (common.ConfigurationProvider, error) {
	//provider := common.DefaultConfigProvider()
	//provider, err := common.ConfigurationProviderFromFile("./oci-config", "")
	provider, err := common.ConfigurationProviderFromFileWithProfile(configPath, profile, privateKeyPassword)
	return provider, err
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
		common.String(config.SubnetDisplayName),
		common.String("10.0.0.0/24"),
		common.String("subnetdns"),
		common.String(config.AvailabilityDomain))
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
		displayName = common.String(config.SubnetDisplayName)
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
	printf("开始创建Subnet（没有可用的Subnet，或指定的Subnet不存在）\n")
	// 子网名称为空，以当前时间为名称创建子网
	if *displayName == "" {
		displayName = common.String(time.Now().Format("subnet-20060102-1504"))
	}
	request := core.CreateSubnetRequest{}
	//request.AvailabilityDomain = availableDomain //省略此属性创建区域性子网(regional subnet)，提供此属性创建特定于可用性域的子网。建议创建区域性子网。
	request.CompartmentId = &config.CompartmentID
	request.CidrBlock = cidrBlock
	request.DisplayName = displayName
	request.DnsLabel = dnsLabel
	request.RequestMetadata = helpers.GetRequestMetadataWithDefaultRetryPolicy()

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
		SecurityListId: common.String(r.SecurityListIds[0]),
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
		SecurityListId: common.String(r.SecurityListIds[0]),
	}

	updateReq.IngressSecurityRules = newRules

	_, err = c.UpdateSecurityList(ctx, updateReq)
	if err != nil {
		return
	}
	printf("Subnet创建成功: %s\n", *r.Subnet.DisplayName)
	subnet = r.Subnet
	return
}

// 列出指定虚拟云网络 (VCN) 中的所有子网，如果该 VCN 不存在会创建 VCN
func listSubnets(ctx context.Context, c core.VirtualNetworkClient, vcnID *string) (subnets []core.Subnet, err error) {
	request := core.ListSubnetsRequest{
		CompartmentId: &config.CompartmentID,
		VcnId:         vcnID,
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
	displayName := common.String(config.VcnDisplayName)
	if len(vcnItems) > 0 && *displayName == "" {
		vcn = vcnItems[0]
		return vcn, err
	}
	for _, element := range vcnItems {
		if *element.DisplayName == config.VcnDisplayName {
			// VCN already created, return it
			vcn = element
			return vcn, err
		}
	}
	// create a new VCN
	printf("开始创建VCN（没有可用的VCN，或指定的VCN不存在）\n")
	if *displayName == "" {
		displayName = common.String(time.Now().Format("vcn-20060102-1504"))
	}
	request := core.CreateVcnRequest{}
	request.CidrBlock = common.String("10.0.0.0/16")
	request.CompartmentId = common.String(config.CompartmentID)
	request.DisplayName = displayName
	request.DnsLabel = common.String("vcndns")
	r, err := c.CreateVcn(ctx, request)
	if err != nil {
		return vcn, err
	}
	printf("VCN创建成功: %s\n", *r.Vcn.DisplayName)
	vcn = r.Vcn
	return vcn, err
}

// 列出所有虚拟云网络 (VCN)
func listVcns(ctx context.Context, c core.VirtualNetworkClient) ([]core.Vcn, error) {
	request := core.ListVcnsRequest{
		CompartmentId: &config.CompartmentID,
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
		CompartmentId: &config.CompartmentID,
		VcnId:         vcnID,
	}

	listGWRespone, err := c.ListInternetGateways(ctx, listGWRequest)
	if err != nil {
		printf("Internet gateway list error: %s\n", err.Error())
		return gateway, err
	}

	if len(listGWRespone.Items) >= 1 {
		//Gateway with name already exists
		gateway = listGWRespone.Items[0]
	} else {
		//Create new Gateway
		printf("开始创建Internet网关\n")
		enabled := true
		createGWDetails := core.CreateInternetGatewayDetails{
			CompartmentId: &config.CompartmentID,
			IsEnabled:     &enabled,
			VcnId:         vcnID,
		}

		createGWRequest := core.CreateInternetGatewayRequest{CreateInternetGatewayDetails: createGWDetails}

		createGWResponse, err := c.CreateInternetGateway(ctx, createGWRequest)

		if err != nil {
			printf("Internet gateway create error: %s\n", err.Error())
			return gateway, err
		}
		gateway = createGWResponse.InternetGateway
		printf("Internet网关创建成功: %s\n", *gateway.DisplayName)
	}
	return gateway, err
}

// 创建或者获取路由表
func createOrGetRouteTable(c core.VirtualNetworkClient, gatewayID, VcnID *string) (routeTable core.RouteTable, err error) {
	//List Route Table
	listRTRequest := core.ListRouteTablesRequest{
		CompartmentId: &config.CompartmentID,
		VcnId:         VcnID,
	}
	var listRTResponse core.ListRouteTablesResponse
	listRTResponse, err = c.ListRouteTables(ctx, listRTRequest)
	if err != nil {
		printf("Route table list error: %s\n", err.Error())
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
			printf("路由表未添加规则，开始添加Internet路由规则\n")
			updateRTDetails := core.UpdateRouteTableDetails{
				RouteRules: []core.RouteRule{rr},
			}

			updateRTRequest := core.UpdateRouteTableRequest{
				RtId:                    listRTResponse.Items[0].Id,
				UpdateRouteTableDetails: updateRTDetails,
			}
			var updateRTResponse core.UpdateRouteTableResponse
			updateRTResponse, err = c.UpdateRouteTable(ctx, updateRTRequest)
			if err != nil {
				printf("Error updating route table: %s\n", err)
				return
			}
			printf("Internet路由规则添加成功\n")
			routeTable = updateRTResponse.RouteTable
		}

	} else {
		//No default route table found
		printf("Error could not find VCN default route table, VCN OCID: %s Could not find route table.\n", *VcnID)
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
		err = fmt.Errorf("未找到[%s %s]的镜像, 或该镜像不支持[%s]", config.OperatingSystem, config.OperatingSystemVersion, config.Shape)
	}
	return
}

// 列出所有符合条件的系统镜像
func listImages(ctx context.Context, c core.ComputeClient) ([]core.Image, error) {
	request := core.ListImagesRequest{
		CompartmentId:          common.String(config.CompartmentID),
		OperatingSystem:        common.String(config.OperatingSystem),
		OperatingSystemVersion: common.String(config.OperatingSystemVersion),
		Shape:                  common.String(config.Shape),
	}
	r, err := c.ListImages(ctx, request)
	return r.Items, err
}

// ListShapes Lists the shapes that can be used to launch an instance within the specified compartment.
func listShapes(ctx context.Context, c core.ComputeClient, imageID *string) []core.Shape {
	request := core.ListShapesRequest{
		CompartmentId: common.String(config.CompartmentID),
		ImageId:       imageID,
	}

	r, err := c.ListShapes(ctx, request)
	helpers.FatalIfError(err)

	if r.Items == nil || len(r.Items) == 0 {
		log.Fatalln("Invalid response from ListShapes")
	}

	return r.Items
}

// 列出符合条件的可用性域
func ListAvailabilityDomains() ([]identity.AvailabilityDomain, error) {
	c, err := identity.NewIdentityClientWithConfigurationProvider(provider)
	if err != nil {
		return nil, err
	}
	setProxyOrNot(&c.BaseClient)
	req := identity.ListAvailabilityDomainsRequest{}
	compartmentID, err := provider.TenancyOCID()
	if err != nil {
		return nil, err
	}
	req.CompartmentId = common.String(compartmentID)
	resp, err := c.ListAvailabilityDomains(context.Background(), req)
	return resp.Items, err
}

func ListInstances(ctx context.Context, c core.ComputeClient) ([]core.Instance, error) {
	compartmentID, err := provider.TenancyOCID()
	if err != nil {
		return nil, err
	}
	req := core.ListInstancesRequest{
		CompartmentId: &compartmentID,
	}
	resp, err := c.ListInstances(ctx, req)
	return resp.Items, err
}

func ListVnicAttachments(ctx context.Context, c core.ComputeClient, instanceId *string) ([]core.VnicAttachment, error) {
	compartmentID, err := provider.TenancyOCID()
	if err != nil {
		return nil, err
	}
	req := core.ListVnicAttachmentsRequest{CompartmentId: &compartmentID}
	if instanceId != nil && *instanceId != "" {
		req.InstanceId = instanceId
	}
	resp, err := c.ListVnicAttachments(ctx, req)
	return resp.Items, err
}

func GetVnic(ctx context.Context, c core.VirtualNetworkClient, vnicID *string) (core.Vnic, error) {
	req := core.GetVnicRequest{VnicId: vnicID}
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
		RequestMetadata:    helpers.GetRequestMetadataWithDefaultRetryPolicy(),
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

func printf(format string, a ...interface{}) {
	fmt.Printf("%s ", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Printf(format, a...)
}

// 根据实例OCID获取公共IP
func getInstancePublicIps(ctx context.Context, computeClient core.ComputeClient, networkClient core.VirtualNetworkClient, instanceId *string) (ips []string, err error) {
	// 多次尝试，避免刚抢购到实例，实例正在预配获取不到公共IP。
	for i := 0; i < 20; i++ {
		vnicAttachments, attachmentsErr := ListVnicAttachments(ctx, computeClient, instanceId)
		if attachmentsErr != nil {
			err = errors.New("获取失败")
			continue
		}
		if len(vnicAttachments) > 0 {
			for _, vnicAttachment := range vnicAttachments {
				vnic, vnicErr := GetVnic(ctx, networkClient, vnicAttachment.VnicId)
				if vnicErr != nil {
					printf("GetVnic error: %s\n", vnicErr.Error())
					continue
				}
				ips = append(ips, *vnic.PublicIp)
			}
			return
		}
		time.Sleep(3 * time.Second)
	}
	return
}

func sendMessage(name, text string) {
	if token != "" && chat_id != "" {
		data := url.Values{
			"parse_mode": {"Markdown"},
			"chat_id":    {chat_id},
			"text":       {"*甲骨文通知*\n名称: " + name + "\n" + "内容: " + text},
		}
		req, err := http.NewRequest(http.MethodPost, sendMessageUrl, strings.NewReader(data.Encode()))
		if err != nil {
			printf("\033[1;31mNewRequest Error: \033[0m%s\n", err.Error())
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		client := common.BaseClient{HTTPClient: &http.Client{}}
		setProxyOrNot(&client)

		resp, err := client.HTTPClient.Do(req)
		if err != nil {
			printf("\033[1;31mTelegram 消息提醒发送失败, Error: \033[0m%s\n", err.Error())
		} else {
			if resp.StatusCode != 200 {
				bodyBytes, err := ioutil.ReadAll(resp.Body)
				var error string
				if err != nil {
					error = err.Error()
				} else {
					error = string(bodyBytes)
				}
				printf("\033[1;31mTelegram 消息提醒发送失败, Error: \033[0m%s\n", error)
			}
		}

	}
}

func setProxyOrNot(client *common.BaseClient) {
	if proxy != "" {
		proxyURL, err := url.Parse(proxy)
		if err != nil {
			fmt.Println(err)
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
	var chineseState string
	switch state {
	case core.InstanceLifecycleStateMoving:
		chineseState = "正在移动"
	case core.InstanceLifecycleStateProvisioning:
		chineseState = "正在预配"
	case core.InstanceLifecycleStateRunning:
		chineseState = "正在运行"
	case core.InstanceLifecycleStateStarting:
		chineseState = "正在启动"
	case core.InstanceLifecycleStateStopping:
		chineseState = "正在停止"
	case core.InstanceLifecycleStateStopped:
		chineseState = "已停止　"
	case core.InstanceLifecycleStateTerminating:
		chineseState = "正在终止"
	case core.InstanceLifecycleStateTerminated:
		chineseState = "已终止　"
	default:
		chineseState = string(state)
	}
	return chineseState
}
