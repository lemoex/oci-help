## 获取用户 OCID 和租户 OCID
登录甲骨文网站
1. 点击右上角头像 -> 点击类似 oracleidentitycloudservice/xxx@xx.com 的条目，进入后即可看到用户的 OCID。
2. 点击右上角头像 -> 点击类似 租户:xxx 的条目, 进入页面后即可获取租户的 OCID。

## 获取其它参数
### 方法1
登录甲骨文网站，创建实例--保存为堆栈(Save as stack),下载 Terraform 配置，解压得到 main.tf 用文本编辑器打开即可。
### 方法2
登录甲骨文网站，创建实例，按F12打开浏览器开发者工具，点击创建实例发起请求，在开发者工具的网络中找到对应的网络请求获取参数。

## 安装并配置 OCI
### 安装
#### Linux
```
bash -c "$(curl -L https://raw.githubusercontent.com/oracle/oci-cli/master/scripts/install/install.sh)"
```
#### macOS
```
brew install oci-cli
```
一路回车即可，当出现 ===> Modify profile to update your $PATH and enable shell/tab completion now? (Y/n): 时，输入y并回车。  
安装完成后执行下面命令
```
# 重新加载环境变量
. ~/.bashrc
# 查看 oci 版本
oci -v
```

### 配置
执行下面命令配置 OCI
```
oci setup config
```
当遇到下面几步时需要按要求输入相应的内容，其他步骤直接回车即可。  
> Enter a user OCID: # 输入你的用户ocid  
> Enter a tenancy OCID: # 输入你租户ocid  
> Enter a region by index or name  # 选择你帐号所在区域，输入编号回车  
> Do you want to generate a new API Signing RSA key pair? ...: y  # 输入y,并回车  

打开甲骨文网站，点击右上角头像 -> 用户设置 -> API 密钥 -> 添加API 密钥，将下面命令查看公钥的全部内容复制添加即可。
```
# 查看生成的公钥
cat ~/.oci/oci_api_key_public.pem
```
执行下面命令，检查配置是否正确。
```
oci iam availability-domain list
```

## 开始创建实例
根据系统类型下载对应的可执行文件 https://github.com/lemoex/oci-help/releases/latest
解压后，执行下面命令运行程序
```
./oci-help
```
输入 2 并回车，根据第一步获取的参数，输入后即可开始创建实例。

## Telegram 消息提醒配置
通过 [BotFather](https://t.me/BotFather) 创建一个 Bot 获取 token，
通过 [IDBot](https://t.me/myidbot) 获取用户 id。  
执行下面命令运行程序，选择第 3 项，接着选择第 1 项设置 token 和用户 id，输入 token 和用户 id 即可。
```
./oci-help
```
