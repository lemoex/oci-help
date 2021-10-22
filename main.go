package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"gopkg.in/ini.v1"
)

var configFile = "./oci-help.ini"
var cfg *ini.File
var defSec *ini.Section
var tg_token string
var tg_chatId string
var name string

type Result struct {
	Status  json.Number `json:"status"`
	Message string      `json:"message"`
	Data    Data        `json:"data"`
}
type Data struct {
	InstanceName string `json:"display-name"`
	Shape        string `json:"shape"`
}

type Node struct {
	Name     string `ini:"name"`
	ADId     string `ini:"ad_id"`
	ImageId  string `ini:"image_id"`
	SubnetId string `ini:"subnet_id"`
	KeyPub   string `ini:"key_pub"`
	Tenancy  string `ini:"tenancy_id"`
	Shape    string `ini:"shape"`
	CPU      string `ini:"cpu_num"`
	RAM      string `ini:"ram_num"`
	HD       string `ini:"hd_num"`
	MinTime  int    `ini:"min_time"`
	MaxTime  int    `ini:"max_time"`
}

func main() {
	_, Error := os.Stat(configFile)
	if os.IsNotExist(Error) {
		os.Create(configFile)
	}
	cfg, _ = ini.Load(configFile)
	defSec = cfg.Section("")
	tg_token = defSec.Key("token").Value()
	tg_chatId = defSec.Key("chat_id").Value()
	rand.Seed(time.Now().UnixNano())
	setCloseHandler()
	mainMenu()
}

func mainMenu() {
	cmdClear()
	fmt.Printf("\n\033[1;32;40m%s\033[0m\n\n", "欢迎使用甲骨文实例抢购脚本")
	fmt.Printf("\033[1;36;40m%s\033[0m %s\n", "1.", "历史抢购实例任务")
	fmt.Printf("\033[1;36;40m%s\033[0m %s\n", "2.", "新建抢购实例任务")
	fmt.Printf("\033[1;36;40m%s\033[0m %s\n", "3.", "Telegram Bot 消息提醒")
	fmt.Println("")
	fmt.Print("请选择[1-3]: ")
	var num int
	fmt.Scanln(&num)
	switch num {
	case 1:
		loadNode()
	case 2:
		addNode()
	case 3:
		setTelegramBot()
	}
}

func loadNode() {
	cmdClear()
	sections := cfg.Sections()
	fmt.Printf("\n\033[1;32;40m%s\033[0m\n\n", "历史抢购实例任务")
	for i := 1; i < len(sections); i++ {
		fmt.Printf("\033[1;36;40m%d.\033[0m %s\n", i, sections[i].Name())
	}
	fmt.Printf("\n\033[1;36;40m%d.\033[0m %s\n", 0, "返回主菜单")
	var num int
	fmt.Print("\n请输入序号, 开始抢购实例: ")
	fmt.Scanln(&num)
	if num <= 0 || num >= len(sections) {
		mainMenu()
		return
	}
	section := sections[num]
	node := new(Node)
	err := section.MapTo(node)
	if err != nil {
		fmt.Println("MapTo failed: ", err)
		return
	}
	launchInstance(node)
}

func addNode() {
	cmdClear()
	var (
		name       string
		ad_id      string
		image_id   string
		subnet_id  string
		key_pub    string
		tenancy_id string
		shape      string
		cpu_num    string
		ram_num    string
		hd_num     string
		min_time   string
		max_time   string
	)
	fmt.Printf("\n\033[1;32;40m%s\033[0m\n\n", "新建抢购实例任务, 请按要求输入以下参数")
	fmt.Print("请随便输入一个名称(不能有空格): ")
	fmt.Scanln(&name)
	fmt.Print("请输入[availabilityDomain|availability_domain]: ")
	fmt.Scanln(&ad_id)
	fmt.Print("请输入[imageId|source_id]: ")
	fmt.Scanln(&image_id)
	fmt.Print("请输入[subnetId|subnet_id]: ")
	fmt.Scanln(&subnet_id)
	fmt.Print("请输入[ssh_authorized_keys]: ")
	reader := bufio.NewReader(os.Stdin)
	key_pub, _ = reader.ReadString('\n')
	key_pub = strings.TrimSuffix(key_pub, "\n")
	fmt.Print("请输入[compartmentId|compartment_id]: ")
	fmt.Scanln(&tenancy_id)
	fmt.Print("请输入[shape]: ")
	fmt.Scanln(&shape)
	fmt.Print("请输入CPU个数: ")
	fmt.Scanln(&cpu_num)
	fmt.Print("请输入内存大小(GB): ")
	fmt.Scanln(&ram_num)
	fmt.Print("请输入引导卷大小(GB): ")
	fmt.Scanln(&hd_num)
	fmt.Print("请输入最小间隔时间(秒): ")
	fmt.Scanln(&min_time)
	fmt.Print("请输入最大间隔时间(秒): ")
	fmt.Scanln(&max_time)

	section := cfg.Section(name)
	section.Key("name").SetValue(name)
	section.NewKey("ad_id", ad_id)
	section.NewKey("image_id", image_id)
	section.NewKey("subnet_id", subnet_id)
	section.NewKey("key_pub", key_pub)
	section.NewKey("tenancy_id", tenancy_id)
	section.NewKey("shape", shape)
	section.NewKey("cpu_num", cpu_num)
	section.NewKey("ram_num", ram_num)
	section.NewKey("hd_num", hd_num)
	section.Key("min_time").SetValue(min_time)
	section.Key("max_time").SetValue(max_time)
	cfg.SaveTo(configFile)

	node := new(Node)
	err := section.MapTo(node)
	if err != nil {
		fmt.Println("MapTo failed: ", err)
		return
	}
	launchInstance(node)
}

func setTelegramBot() {
	cmdClear()
	fmt.Printf("\n\033[1;32;40m%s\033[0m\n\n", "Telegram Bot 消息提醒配置")
	fmt.Println("Telegram Bot Token:", tg_token)
	fmt.Println("Telegram User ID:", tg_chatId)
	fmt.Printf("\n\033[1;36;40m%s\033[0m %s\n", "1.", "设置token和用户id")
	fmt.Printf("\n\033[1;36;40m%s\033[0m %s\n", "0.", "返回主菜单")
	fmt.Print("\n请选择[0-1]: ")
	var num int
	fmt.Scanln(&num)
	switch num {
	case 1:
		fmt.Print("请输入 Telegram Bot Token: ")
		fmt.Scanln(&tg_token)
		fmt.Print("请输入 Telegram User ID: ")
		fmt.Scanln(&tg_chatId)
		defSec.Key("token").SetValue(tg_token)
		defSec.Key("chat_id").SetValue(tg_chatId)
		cfg.SaveTo(configFile)
		fmt.Println("设置成功")
		mainMenu()
	default:
		mainMenu()
	}
}

func sendMessage(name, text string) {
	tg_url := "https://api.telegram.org/bot" + tg_token + "/sendMessage"
	urlValues := url.Values{
		"parse_mode": {"Markdown"},
		"chat_id":    {tg_chatId},
		"text":       {"*甲骨文通知*\n名称: " + name + "\n" + "内容: " + text},
	}
	cli := http.Client{Timeout: 10 * time.Second}
	resp, err := cli.PostForm(tg_url, urlValues)
	if err != nil {
		printYellow("Telegram 消息提醒发送失败: " + err.Error())
		return
	}
	if resp.StatusCode != 200 {
		body, err := ioutil.ReadAll(resp.Body)
		if err == nil {
			bodyStr := string(body)
			printYellow("Telegram 消息提醒发送失败: " + bodyStr)
		}
	}
}

func launchInstance(node *Node) {
	name = node.Name
	text := "开始抢购实例"
	printGreen(text)
	sendMessage(node.Name, text)
	cmd := "oci"
	args := []string{
		"compute", "instance", "launch",
		"--availability-domain", node.ADId,
		"--image-id", node.ImageId,
		"--subnet-id", node.SubnetId,
		"--metadata", `{"ssh_authorized_keys": "` + node.KeyPub + `"}`,
		"--compartment-id", node.Tenancy,
		"--shape", node.Shape,
		"--shape-config", `{"ocpus":` + node.CPU + `,"memory_in_gbs":` + node.RAM + `}`,
		"--boot-volume-size-in-gbs", node.HD,
		"--assign-public-ip", "true",
		"--is-pv-encryption-in-transit-enabled", "true",
	}

	for {
		printYellow("正在尝试新建实例......")
		out, err := exec.Command(cmd, args...).CombinedOutput()
		ret := string(out)
		if err != nil && out == nil {
			text = "出现异常: " + err.Error()
			printRed(text)
			sendMessage(node.Name, text)
			return
		}
		pos := strings.Index(ret, "{")
		if pos != -1 {
			ret = ret[pos:]
		}
		var result Result
		err = json.Unmarshal([]byte(ret), &result)
		if err != nil {
			text = "出现异常: " + ret
			printRed(text)
			continue
		}
		switch result.Status {
		case "500", "429":
			printNone(result.Message)
		default:
			if result.Data != (Data{}) {
				text = "抢购成功, 实例名称: [" + result.Data.InstanceName + "]"
				printGreen(text)
				sendMessage(node.Name, text)
				return
			}
			text = "抢购失败, " + result.Message
			printRed(text)
			sendMessage(node.Name, text)
			return
		}
		random := random(node.MinTime, node.MaxTime)
		time.Sleep(time.Duration(random) * time.Second)
	}
}

func random(min, max int) int {
	if min == 0 || max == 0 {
		return 1
	}
	if min >= max {
		return max
	}
	return rand.Intn(max-min) + min
}

func setCloseHandler() {
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Printf("\033[1;33;40m%s\033[0m\n", "已停止")
		if name != "" {
			sendMessage(name, "已停止")
		}
		os.Exit(0)
	}()

}

func cmdClear() {
	cmd := exec.Command("clear")
	cmd.Stdout = os.Stdout
	cmd.Run()
}

func printRed(str string) {
	fmt.Print(time.Now().Format("[2006-01-02 15:04:05] "))
	fmt.Printf("\033[1;31;40m%s\033[0m\n", str)
}
func printGreen(str string) {
	fmt.Print(time.Now().Format("[2006-01-02 15:04:05] "))
	fmt.Printf("\033[1;32;40m%s\033[0m\n", str)
}
func printYellow(str string) {
	fmt.Print(time.Now().Format("[2006-01-02 15:04:05] "))
	fmt.Printf("\033[1;33;40m%s\033[0m\n", str)
}
func printNone(str string) {
	fmt.Print(time.Now().Format("[2006-01-02 15:04:05] "))
	fmt.Printf("%s\n", str)
}
