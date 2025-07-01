# 甲骨文实例管理工具使用教程

## 获取程序
- 如果需要在本地电脑运行该程序，根据自己电脑的CPU架构类型，打开下面的 `下载地址` 下载对应的文件即可。如果需要在服务器上运行该程序，那么根据服务器的CPU架构类型选择对应的文件下载到服务器即可。

- 下载完成后解压压缩包，可以得到可执行程序 (Windows系统: `oci-help.exe` , 其他操作系统: `oci-help` ) 和 配置文件 `oci-help.ini` 。

> 下载地址: https://github.com/lemoex/oci-help/releases/latest


## 获取配置信息
![image](https://github.com/lemoex/oci-help/raw/main/doc/1.png)
![image](https://github.com/lemoex/oci-help/raw/main/doc/2.png)
![image](https://github.com/lemoex/oci-help/raw/main/doc/3.png)
![image](https://github.com/lemoex/oci-help/raw/main/doc/4.png)
![image](https://github.com/lemoex/oci-help/raw/main/doc/5.png)
![image](https://github.com/lemoex/oci-help/raw/main/doc/6.png)


## 编辑配置文件
用文本编辑器打开在第一步获取到的 `oci-help.ini` 文件，进行如下配置:

![image](https://github.com/lemoex/oci-help/raw/main/doc/7.png)
![image](https://github.com/lemoex/oci-help/raw/main/doc/8.png)

## Telegram 消息提醒配置
![image](https://github.com/lemoex/oci-help/raw/main/doc/9.png)

> BotFather: https://t.me/BotFather    
> IDBot: https://t.me/myidbot


## 运行程序
```bash
# 前台运行程序
./oci-help

# 前台运行需要一直开着终端窗口，可以在 Screen 中运行程序，以实现断开终端窗口后一直运行。
# 创建 Screen 终端
screen -S oci-help 
# 在 Screen 中运行程序
./oci-help
# 离开 Screen 终端
按下 Ctrl 键不松，依次按字母 A 键和 D 键。或者直接关闭终端窗口也可以。
# 查看已创建的 Screen 终端
screen -ls
# 重新连接 Screen 终端
screen -r oci-help
```


## 🎉 感谢赞助

本项目 CDN 加速及安全防护由 [Tencent EdgeOne](https://edgeone.ai/zh?from=github "Tencent EdgeOne") 赞助

[![Tencent EdgeOne](https://edgeone.ai/media/34fe3a45-492d-4ea4-ae5d-ea1087ca7b4b.png)](https://edgeone.ai/zh?from=github)

