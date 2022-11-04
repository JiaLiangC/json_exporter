# Json Exporter
<p align="center">
  <a href="" rel="noopener">
</p>

<h3 align="center">Json Exporter</h3>

<div align="center">

[![Status](https://img.shields.io/badge/status-active-success.svg)]()
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](/LICENSE)

</div>

---

<p align="center">   Prometheus exporter that collects all components that expose monitoring data in the form of http json
    <br> 
</p>

[English](./README.md) | [中文](./README.zh-CN.md)
## 📝 Table of Contents
- [About](#about)
- [Usage](#usage)
- [Built Using](#built_using)

## 🧐 About <a name = "about"></a>
As we all know, monitoring is very important for the stable operation and maintenance of components, so do we need to write an exporter for each component?

No, in fact, most components have http interfaces to return their monitoring data in the form of json.
Therefore, only one json exporter needs to be deployed on this machine,
and some configuration is required to collect monitoring data of all components on this machine.

## 🎈 Usage <a name="usage"></a>

#### Global configuration
| Configuration item       | Description                                                     |
|--------------------------|-----------------------------------------------------------------|
| listenAddr               | listening address of exporter                                   |
| whiteListDir             | The global whitelist directory of the collected components      |
| Components （typed array）| All components that need to be collected on the current machine |

#### component configuration item
| processName           | Component process name (empty by default)                |
|-----------------------|----------------------------------------------------------|
| port                  | component port (empty by default)                        |
| name                  | component name (empty by default)                        |
| whiteListDir          | component whitelist directory (empty by default)         |  
| allowRecursiveParse   | Whether to allow recursive collection (empty by default) | 
| allowMetricsWhiteList | Whether to enable whitelist  (empty by default)          |
| jmxUrlSuffix          | The jmx url address of the component (default /jmx)      |

### Configuration example

**Basic configuration**：Configure the monitoring data http port, process name, and component name of the component to be collected (used to label the monitoring data), it can automatically discover the process on the deployed machine, and collect monitoring data from the port you configured .
You can configure multiple components that need to be collected.

**Basic configuration example**：
The following is an example configuration collected HiveServer2 HbaseMaster HbaseRegionServer YarnResourceManager YarnNodeManager HadoopNameNode
HadoopDataNode ImpalaImpalad ImpalaCatalogd jmx data for these components.
```
listenAddr: 0.0.0.0:9308
Components:
  - { name: "HiveServer2",port: 10002,processName: "Dproc_hiveserver2" }
  - { name: "HbaseMaster",port: 16010,processName: "org.apache.hadoop.hbase.master.HMaster"}
  - { name: "HbaseRegionServer",port: 16030,processName: "org.apache.hadoop.hbase.regionserver.HRegionServer"}
  - { name: "YarnResourceManager",port: 8088,processName: "org.apache.hadoop.yarn.server.resourcemanager.ResourceManager"}
  - { name: "YarnNodeManager",port: 8042,processName: "Dproc_nodemanager"}
  - { name: "HadoopNameNode",port: 50070,processName: "org.apache.hadoop.hdfs.server.namenode.NameNode"}
  - { name: "HadoopDataNode",port: 1022,processName: "Dproc_datanode"}
  - { name: "ImpalaImpalad",port: 25000,processName: "impalad"}
  - { name: "ImpalaCatalogd",port: 25020,processName: "catalogd"}
```

---

### whitelist
#### Why do you need a whitelist?
Since many components expose a lot of monitoring data items, if all of them are collected, it will put a lot of pressure on the storage side. Therefore, by using the whitelist configuration, only the required monitoring data can be collected, which greatly reduces the pressure on the time series database.
If you need to collect all indicators, you can also manually turn off the whitelist in the configuration item.

#### How to configure the whitelist:
**1.Configure the whitelist file directory**

##### 1.1 Global whitelist directory
A global whitelist directory can be configured for all components, using the whiteListDir configuration item
```
listenAddr: 0.0.0.0:9308
whiteListDir: "/usr/local/expoter/conf/"
Components:
- { name: "HiveServer2",port: 10002,processName: "Dproc_hiveserver2" }
- { name: "HbaseMaster",port: 16010,processName: "org.apache.hadoop.hbase.master.HMaster"}
```

##### 1.2 Component whitelist file directory
You can also configure a whitelist directory for each component individually. This directory will override the global whitelist directory.
If not configured, the global whitelist directory will be used.
```
listenAddr: 0.0.0.0:9308
whiteListDir: "/usr/local/expoter/conf/"
Components:
- { name: "HiveServer2",port: 10002,processName: "Dproc_hiveserver2", whiteListDir: "/usr/local/expoter/HiveServer2/" }
- { name: "HbaseMaster",port: 16010,processName: "org.apache.hadoop.hbase.master.HMaster"}
```
##### 1.3 Create a whitelist file
The name of the whitelist file is the component {name}.json in the configuration file. For example, the name attribute in the configuration of the above hive component is HiveServer2 ,
then the file name of the whitelist is HiveServer2.json

Example of whitelist file content:

For example, the monitoring data of the request hive server is as follows http://xx.xxx.xx.xx:10002/jmx
```
{
  "beans" : [ {
    "name" : "org.apache.logging.log4j2:type=6aaa5eb0,component=Appenders,name=console",
    "modelerType" : "org.apache.logging.log4j.core.jmx.AppenderAdmin",
    "Name" : "console",
    "ErrorHandler" : "org.apache.logging.log4j.core.appender.DefaultErrorHandler@3bed98b4",
    "Filter" : "null",
    "Layout" : "%d{ISO8601} %5p [%t] %c{2}: %m%n",
    "IgnoreExceptions" : true
  }, {
    "name" : "java.lang:type=OperatingSystem",
    "modelerType" : "sun.management.OperatingSystemImpl",
    "OpenFileDescriptorCount" : 670,
    ...
  },{
    "name" : "java.lang:type=MemoryPool,name=Code Cache",
    "modelerType" : "sun.management.MemoryPoolImpl",
    "Usage" : {
      "committed" : 52690944,
      "init" : 2555904,
      "max" : 251658240,
      "used" : 52043648
    },
    "PeakUsage" : {
      "committed" : 52690944,
      "init" : 2555904,
      "max" : 251658240,
      "used" : 52046400
    }
```
If you want to collect OpenFileDescriptorCount
Then the whitelist file HiveServer2.json is as follows
```
{
"OpenFileDescriptorCount": "Description of this metric"
}
```

---
##### Recursive collection
If you want to collect OpenFileDescriptorCount
Note that if you want to collect the value of "committed": 52690944,
you need to open recursive collection, because the metrics json of many components is relatively large and complex,
and only the data of the first layer of map is collected by default
```
- { name: "HiveServer2",port: 10002,processName: "Dproc_hiveserver2",allowRecursiveParse: "true"}
```

Then the indicator names in the whitelist need to underscore the keys of all their parent maps
```
{
"Usage_committed": "Description of this metric"
}
```
**About recursive collection**：Some metrics of individual components are located at a deep json level, so recursive collection is required. This option is disabled by default and needs to be manually enabled in the configuration file.
The enabled configuration item is allowRecursiveParse
```
- { name: "HiveServer2",port: 10002,processName: "Dproc_hiveserver2",allowRecursiveParse: "true"}
```
---
#### Collection address configuration:
The collection address will collect monitoring data of {componentIp}:port/jmx by default.

HiveServer2 HbaseMaster HbaseRegionServer YarnResourceManager YarnNodeManager HadoopNameNode
HadoopDataNode ImpalaImpalad ImpalaCatalogd For these components, their jmx monitoring data addresses are {componentIp}:port/jmx,
Therefore, no additional configuration of the acquisition address is required.

If the url of the component monitoring data you need to collect is not {componentIp}:port/jmx, you need to configure the jmxUrlSuffix configuration item.
For example, your monitoring data address is {componentIp}:port/metrics, then the configuration is as follows
```
- { name: "HiveServer2",port: 10002,processName: "Dproc_hiveserver2",allowRecursiveParse: "true",jmxUrlSuffix: "/metrics"}
```



## ️ Built Using <a name = "built_using"></a>
- [GOLang](https://go.dev/) - Server Environment

