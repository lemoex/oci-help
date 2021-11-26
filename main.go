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
	"strings"
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
	var configFilePath string
	flag.StringVar(&configFilePath, "config", defConfigFilePath, "配置文件路径")
	flag.StringVar(&configFilePath, "c", defConfigFilePath, "配置文件路径")
	flag.Parse()

	cfg, err := ini.Load(configFilePath)
	helpers.FatalIfError(err)
	sections := cfg.Sections()
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

	fmt.Printf("\n\033[1;32m%s\033[0m\n\n", "欢迎使用甲骨文实例创建工具")
	fmt.Printf("\033[1;36m%s\033[0m %s\n", "1.", "开始创建实例")
	fmt.Printf("\033[1;36m%s\033[0m %s\n", "2.", "获取实例IP地址")
	fmt.Println("")
	fmt.Print("请选择[1-2]: ")
	var num int
	fmt.Scanln(&num)
	switch num {
	case 1:
		CreateInstances(sections, configFilePath)
	case 2:
		ListAllIPs(sections, configFilePath)
	default:
	}

}

func CreateInstances(sections []*ini.Section, configFile string) {
	for _, section := range sections {
		if len(section.ChildSections()) > 0 {
			provider = getProvider(configFile, section.Name(), "")

			printf("\033[1;36m[%s]\033[0m\n", section.Name())

			var SUM, NUM int32 = 0, 0
			sendMessage(section.Name(), "开始创建")

			// 获取可用性域
			AvailabilityDomains := ListAvailabilityDomains()

			for _, child := range section.ChildSections() {
				providerName = child.Name()
				config = Config{}
				err := child.MapTo(&config)
				helpers.FatalIfError(err)

				sum, num := LaunchInstances(AvailabilityDomains)

				SUM = SUM + sum
				NUM = NUM + num

			}

			printf("\033[1;36m[%s], 创建总数: %d, 创建成功 %d , 创建失败 %d\033[0m\n", section.Name(), SUM, NUM, SUM-NUM)

			text := fmt.Sprintf("结束创建。创建总数: %d, 创建成功 %d , 创建失败 %d", SUM, NUM, SUM-NUM)
			sendMessage(section.Name(), text)
		}
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

	printf("\033[1;36m[%s] 开始创建...\033[0m\n", providerName)
	computeClient, err := core.NewComputeClientWithConfigurationProvider(provider)
	helpers.FatalIfError(err)
	setProxyOrNot(&computeClient.BaseClient)
	networkClient, err := core.NewVirtualNetworkClientWithConfigurationProvider(provider)
	helpers.FatalIfError(err)
	setProxyOrNot(&networkClient.BaseClient)

	ctx := context.Background()
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
	subnet := CreateOrGetNetworkInfrastructure(ctx, networkClient)
	printf("子网: %s\n", *subnet.DisplayName)
	request.CreateVnicDetails = &core.CreateVnicDetails{SubnetId: subnet.Id}
	// Get a image.
	image, err := GetImage(ctx, computeClient)
	helpers.FatalIfError(err)
	printf("系统镜像: %s\n", *image.DisplayName)
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

	minTime := config.MinTime
	maxTime := config.MaxTime

	SKIP_RETRY_MAP := make(map[int32]bool)
	var usableAdsTemp = make([]identity.AvailabilityDomain, 0)

	retry := config.Retry   // 重试次数
	var failTimes int32 = 0 // 失败次数

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

		printf("\033[1;36m[%s] 第 %d 个实例正在创建, AD: %s\033[0m\n", providerName, pos+1, *adName)

		request.AvailabilityDomain = adName
		createResp, err := computeClient.LaunchInstance(ctx, request)

		if err == nil {
			// 创建实例成功
			SUCCESS = true
			num++ //成功个数+1

			printf("\033[1;32m[%s] 第 %d 个实例创建成功, 实例名称: %s\033[0m\n", providerName, pos+1, *createResp.Instance.DisplayName)
			if EACH {
				sendMessage(providerName, "创建成功，实例名称: "+*createResp.DisplayName)
			}

			ips := getInstancePublicIps(ctx, computeClient, networkClient, createResp.Instance.Id)
			strIps := strings.Join(ips, ",")
			printf("\033[1;32m[%s] 实例名称: %s, IP: %s\033[0m\n", providerName, *createResp.Instance.DisplayName, strIps)
			if EACH {
				sendMessage(providerName, "实例名称: "+*createResp.DisplayName+", IP: "+strIps)
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
			//fmt.Println("IsErrorRetryableByDefault", isRetryable)
			isNetErr := common.IsNetworkError(err)
			servErr, isServErr := common.IsServiceError(err)
			if isNetErr || (isServErr && (servErr.GetHTTPStatusCode() == 409 || servErr.GetHTTPStatusCode() == 429 || (500 <= servErr.GetHTTPStatusCode() && servErr.GetHTTPStatusCode() < 505))) {
				// 可重试
				if isServErr {
					errInfo = servErr.GetMessage()
				}
				printf("\033[1;31m[%s] 第 %d 个实例创建失败, Error: \033[0m%s\n", providerName, pos+1, errInfo)

				SKIP_RETRY = false
				if AD_NOT_FIXED && !EACH_AD {
					SKIP_RETRY_MAP[adIndex-1] = false
				}

			} else {
				// 无需重试
				if isServErr {
					errInfo = servErr.GetMessage()
				}
				printf("\033[1;31m[%s] 第 %d 个实例创建失败, Error: \033[0m%s\n", providerName, pos+1, errInfo)
				if EACH {
					sendMessage(providerName, "创建失败，Error: "+errInfo)
				}

				SKIP_RETRY = true
				if AD_NOT_FIXED && !EACH_AD {
					SKIP_RETRY_MAP[adIndex-1] = true
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

func ListAllIPs(sections []*ini.Section, configFile string) {
	IPsFilePath := IPsFilePrefix + "-" + time.Now().Format("2006-01-02-150405.txt")
	_, err := os.Stat(IPsFilePath)
	if err != nil && os.IsNotExist(err) {
		os.Create(IPsFilePath)
	}

	printf("获取实例IP地址...\n")
	for _, section := range sections {

		if len(section.ChildSections()) > 0 {
			provider = getProvider(configFile, section.Name(), "")
			ListInstancesIPs(IPsFilePath, section.Name())
		}

	}
	printf("获取实例IP地址完成，请查看文件 %s\n", IPsFilePath)
}

func ListInstancesIPs(filePath string, sectionName string) {
	ctx := context.Background()
	computeClient, err := core.NewComputeClientWithConfigurationProvider(provider)
	helpers.FatalIfError(err)
	setProxyOrNot(&computeClient.BaseClient)
	netClient, err := core.NewVirtualNetworkClientWithConfigurationProvider(provider)
	helpers.FatalIfError(err)
	setProxyOrNot(&netClient.BaseClient)

	vnicAttachments, err := ListVnicAttachments(ctx, computeClient, nil)
	if err != nil {
		printf("ListVnicAttachments Error: %s\n", err.Error())
	}

	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, os.ModeAppend)
	if err != nil {
		printf("打开文件失败, Error: %s\n", err.Error())
		return
	}
	_, err = io.WriteString(file, "["+sectionName+"]\n")
	if err != nil {
		printf("%s\n", err.Error())
	}
	for _, vnicAttachment := range vnicAttachments {

		vnic, err := GetVnic(ctx, netClient, vnicAttachment.VnicId)
		if err != nil {
			printf("IP地址获取失败, %s\n", err.Error())
			continue
		}
		printf("实例: %s, IP: %s\n", *vnic.DisplayName, *vnic.PublicIp)
		_, err = io.WriteString(file, "实例: "+*vnic.DisplayName+", IP: "+*vnic.PublicIp+"\n")
		if err != nil {
			printf("写入文件失败, Error: %s\n", err.Error())
		}
	}
	_, err = io.WriteString(file, "\n")
	if err != nil {
		printf("%s\n", err.Error())
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
	subnet := CreateOrGetNetworkInfrastructure(ctx, networkClient)
	fmt.Println("subnet created")
	request.CreateVnicDetails = &core.CreateVnicDetails{SubnetId: subnet.Id}

	// get a image
	image := listImages(ctx, c)[0]
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
		terminateInstance(ctx, c, createResp.Id)

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

func getProvider(configPath, profile, privateKeyPassword string) common.ConfigurationProvider {
	//provider := common.DefaultConfigProvider()
	//provider, err := common.ConfigurationProviderFromFile("./oci-config", "")
	provider, err := common.ConfigurationProviderFromFileWithProfile(configPath, profile, privateKeyPassword)
	helpers.FatalIfError(err)
	return provider
}

// 创建或获取基础网络设施
func CreateOrGetNetworkInfrastructure(ctx context.Context, c core.VirtualNetworkClient) core.Subnet {
	vcn := createOrGetVcn(ctx, c)
	gateway := createOrGetInternetGateway(c, vcn.Id)
	createOrGetRouteTable(c, gateway.Id, vcn.Id)
	subnet := createOrGetSubnetWithDetails(
		ctx, c, vcn.Id,
		common.String(config.SubnetDisplayName),
		common.String("10.0.0.0/24"),
		common.String("subnetdns"),
		common.String(config.AvailabilityDomain))

	return subnet
}

// CreateOrGetSubnetWithDetails either creates a new Virtual Cloud Network (VCN) or get the one already exist
// with detail info
func createOrGetSubnetWithDetails(ctx context.Context, c core.VirtualNetworkClient, vcnID *string,
	displayName *string, cidrBlock *string, dnsLabel *string, availableDomain *string) core.Subnet {
	subnets := listSubnets(ctx, c, vcnID)

	if displayName == nil {
		displayName = common.String(config.SubnetDisplayName)
	}

	if len(subnets) > 0 && *displayName == "" {
		return subnets[0]
	}

	// check if the subnet has already been created
	for _, element := range subnets {
		if *element.DisplayName == *displayName {
			// find the subnet, return it
			return element
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
	r, err := c.CreateSubnet(ctx, request)
	helpers.FatalIfError(err)
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
	_, pollErr := c.GetSubnet(ctx, pollGetRequest)
	helpers.FatalIfError(pollErr)

	// update the security rules
	getReq := core.GetSecurityListRequest{
		SecurityListId: common.String(r.SecurityListIds[0]),
	}

	getResp, err := c.GetSecurityList(ctx, getReq)
	helpers.FatalIfError(err)

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
	helpers.FatalIfError(err)
	printf("Subnet创建成功: %s\n", *r.Subnet.DisplayName)
	return r.Subnet
}

// 列出指定虚拟云网络 (VCN) 中的所有子网，如果该 VCN 不存在会创建 VCN
func listSubnets(ctx context.Context, c core.VirtualNetworkClient, vcnID *string) []core.Subnet {
	request := core.ListSubnetsRequest{
		CompartmentId: &config.CompartmentID,
		VcnId:         vcnID,
	}
	r, err := c.ListSubnets(ctx, request)
	helpers.FatalIfError(err)
	return r.Items
}

// 创建一个新的虚拟云网络 (VCN) 或获取已经存在的虚拟云网络
func createOrGetVcn(ctx context.Context, c core.VirtualNetworkClient) core.Vcn {
	vcnItems := listVcns(ctx, c)

	displayName := common.String(config.VcnDisplayName)

	if len(vcnItems) > 0 && *displayName == "" {
		return vcnItems[0]
	}

	for _, element := range vcnItems {
		if *element.DisplayName == config.VcnDisplayName {
			// VCN already created, return it
			return element
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
	helpers.FatalIfError(err)
	printf("VCN创建成功: %s\n", *r.Vcn.DisplayName)
	return r.Vcn
}

// 列出所有虚拟云网络 (VCN)
func listVcns(ctx context.Context, c core.VirtualNetworkClient) []core.Vcn {
	request := core.ListVcnsRequest{
		CompartmentId: &config.CompartmentID,
	}
	r, err := c.ListVcns(ctx, request)
	helpers.FatalIfError(err)
	return r.Items
}

// 创建或者获取 Internet 网关
func createOrGetInternetGateway(c core.VirtualNetworkClient, vcnID *string) (gateway core.InternetGateway) {
	ctx := context.Background()
	//List Gateways
	listGWRequest := core.ListInternetGatewaysRequest{
		CompartmentId: &config.CompartmentID,
		VcnId:         vcnID,
	}

	listGWRespone, err := c.ListInternetGateways(ctx, listGWRequest)
	if err != nil {
		printf("Internet gateway list error: %s\n", err.Error())
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
		}
		gateway = createGWResponse.InternetGateway
		printf("Internet网关创建成功: %s\n", *gateway.DisplayName)
	}
	return
}

// 创建或者获取路由表
func createOrGetRouteTable(c core.VirtualNetworkClient, gatewayID, VcnID *string) (routeTable core.RouteTable) {
	ctx := context.Background()
	//List Route Table
	listRTRequest := core.ListRouteTablesRequest{
		CompartmentId: &config.CompartmentID,
		VcnId:         VcnID,
	}

	listRTResponse, err := c.ListRouteTables(ctx, listRTRequest)
	if err != nil {
		printf("Route table list error: %s\n", err.Error())
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

			updateRTResponse, err := c.UpdateRouteTable(ctx, updateRTRequest)
			if err != nil {
				printf("Error updating route table: %s\n", err)
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
	images := listImages(ctx, c)
	if len(images) > 0 {
		image = images[0]
	} else {
		err = fmt.Errorf("未找到[%s %s]的镜像, 或该镜像不支持[%s]", config.OperatingSystem, config.OperatingSystemVersion, config.Shape)
	}
	return
}

// 列出所有符合条件的系统镜像
func listImages(ctx context.Context, c core.ComputeClient) []core.Image {
	request := core.ListImagesRequest{
		CompartmentId:          common.String(config.CompartmentID),
		OperatingSystem:        common.String(config.OperatingSystem),
		OperatingSystemVersion: common.String(config.OperatingSystemVersion),
		Shape:                  common.String(config.Shape),
	}
	r, err := c.ListImages(ctx, request)
	helpers.FatalIfError(err)
	return r.Items
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
func ListAvailabilityDomains() []identity.AvailabilityDomain {
	c, err := identity.NewIdentityClientWithConfigurationProvider(provider)
	helpers.FatalIfError(err)
	setProxyOrNot(&c.BaseClient)
	req := identity.ListAvailabilityDomainsRequest{}
	compartmentID, err := provider.TenancyOCID()
	helpers.FatalIfError(err)
	req.CompartmentId = common.String(compartmentID)
	resp, err := c.ListAvailabilityDomains(context.Background(), req)
	helpers.FatalIfError(err)
	return resp.Items
}

func ListInstances(ctx context.Context, c core.ComputeClient) []core.Instance {
	compartmentID, err := provider.TenancyOCID()
	helpers.FatalIfError(err)
	req := core.ListInstancesRequest{
		CompartmentId: &compartmentID,
	}
	resp, err := c.ListInstances(ctx, req)
	helpers.FatalIfError(err)
	return resp.Items
}

func ListVnicAttachments(ctx context.Context, c core.ComputeClient, instanceId *string) ([]core.VnicAttachment, error) {
	compartmentID, err := provider.TenancyOCID()
	helpers.FatalIfError(err)
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

func listPublicIPs(ctx context.Context, c core.VirtualNetworkClient, ad *string) []core.PublicIp {
	com, err := provider.TenancyOCID()
	helpers.FatalIfError(err)
	req := core.ListPublicIpsRequest{
		Scope:              core.ListPublicIpsScopeAvailabilityDomain,
		CompartmentId:      &com,
		AvailabilityDomain: ad,
	}
	resp, err := c.ListPublicIps(ctx, req)
	helpers.FatalIfError(err)
	return resp.Items
}

// 终止实例
func terminateInstance(ctx context.Context, c core.ComputeClient, id *string) {
	request := core.TerminateInstanceRequest{
		InstanceId:      id,
		RequestMetadata: helpers.GetRequestMetadataWithDefaultRetryPolicy(),
	}

	_, err := c.TerminateInstance(ctx, request)
	helpers.FatalIfError(err)

	fmt.Println("terminating instance")

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
func getInstancePublicIps(ctx context.Context, computeClient core.ComputeClient, networkClient core.VirtualNetworkClient, instanceId *string) (ips []string) {
	var err error
	var vnicAttachments []core.VnicAttachment
	var vnic core.Vnic
	vnicAttachments, err = ListVnicAttachments(ctx, computeClient, instanceId)
	if err != nil {
		printf("ListVnicAttachments error: %s\n", err.Error())
		return
	}
	for _, vnicAttachment := range vnicAttachments {
		vnic, err = GetVnic(ctx, networkClient, vnicAttachment.VnicId)
		if err != nil {
			printf("GetVnic error: %s\n", err.Error())
			continue
		}
		ips = append(ips, *vnic.PublicIp)
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
		helpers.FatalIfError(err)
		client.HTTPClient = &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
			},
		}
	}
}
