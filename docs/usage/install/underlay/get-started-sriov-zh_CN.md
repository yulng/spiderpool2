# SRIOV Quick Start

[**English**](./get-started-sriov.md) | **简体中文**

Spiderpool 可用作 underlay 网络场景下提供固定 IP 的一种解决方案，本文将以 [Multus](https://github.com/k8snetworkplumbingwg/multus-cni)、[Sriov](https://github.com/k8snetworkplumbingwg/sriov-cni) 、[Spiderpool](https://github.com/spidernet-io/spiderpool) 为例，搭建一套完整的 Underlay 网络解决方案，该方案能够满足以下各种功能需求：

* 通过简易运维，应用可分配到固定的 Underlay IP 地址

* Pod 的网卡具有 Sriov 的网络加速功能

* Pod 能够通过 Pod IP、clusterIP、nodePort 等方式通信

## 先决条件

1. 一个 Kubernetes 集群
2. [Helm 工具](https://helm.sh/docs/intro/install/)
3. [支持 SR-IOV 功能的网卡](https://github.com/k8snetworkplumbingwg/sriov-network-device-plugin#supported-sr-iov-nics)

    * 查询网卡 bus-info：

        ```shell
        ~# ethtool -i enp4s0f0np0 |grep bus-info
        bus-info: 0000:04:00.0
        ```

    * 通过 bus-info 查询网卡是否支持 SR-IOV 功能，出现 `Single Root I/O Virtualization (SR-IOV)` 字段表示网卡支持 SR-IOV 功能：

        ```shell
        ~# lspci -s 0000:04:00.0 -v |grep SR-IOV
        Capabilities: [180] Single Root I/O Virtualization (SR-IOV)      
        ```
    
### 安装 Sriov-network-operator

Sriov-network-operator 可以帮助我们自动安装、配置 sriov-cni 和 sriov-device-plugin。

1. 安装 sriov-network-operator

    ```shell
    git clone https://github.com/k8snetworkplumbingwg/sriov-network-operator.git && cd sriov-network-operator/deployment
    helm install -n sriov-network-operator --create-namespace --set operator.resourcePrefix=spidernet.io  --wait sriov-network-operator ./
    ```
   
   > 必须给 sriov 工作节点打上 label: 'node-role.kubernetes.io/worker=""'，sriov-operator 相关组件才会就绪。
   > 
   > sriov-network-operator 默认安装在 sriov-network-operator 命名空间下

2. 配置 sriov-network-operator

    如果 SriovNetworkNodeState CRs 的状态为 `InProgress`, 说明 sriov-operator 正在同步节点状态，等待状态为 `Suncceeded` 说明同步完成。查看 CR, 确认 sriov-network-operator 已经发现节点上支持 SR-IOV 功能的网卡。

    ```shell
    $ kubectl get sriovnetworknodestates.sriovnetwork.openshift.io -n sriov-network-operator node-1 -o yaml
    apiVersion: sriovnetwork.openshift.io/v1
    kind: SriovNetworkNodeState
    spec: ...
    status:
      interfaces:
      - deviceID: "1017"
        driver: mlx5_core
        linkSpeed: 10000 Mb/s
        linkType: ETH
        mac: 04:3f:72:d0:d2:86
        mtu: 1500
        name: enp4s0f0np0
        pciAddress: "0000:04:00.0"
        totalvfs: 8
        vendor: 15b3
      - deviceID: "1017"
        driver: mlx5_core
        linkSpeed: 10000 Mb/s
        linkType: ETH
        mac: 04:3f:72:d0:d2:87
        mtu: 1500
        name: enp4s0f1np1
        pciAddress: "0000:04:00.1"
        totalvfs: 8
        vendor: 15b3
      syncStatus: Succeeded
    ```
   
    从上面可知，节点 `node-1` 上的接口 `enp4s0f0np0` 和 `enp4s0f1np1` 都具有 SR-IOV 功能，并且支持的最大 VF 数量为 8。 下面我们将通过创建 SriovNetworkNodePolicy CRs 来配置 VFs，并且安装 sriov-device-plugin :

    ```shell
    $ cat << EOF | kubectl apply -f -
    apiVersion: sriovnetwork.openshift.io/v1
    kind: SriovNetworkNodePolicy
    metadata:
      name: policy1
      namespace: sriov-network-operator
    spec:
      deviceType: netdevice
      nicSelector:
      pfNames:
      - enp4s0f0np0
      nodeSelector:
        kubernetes.io/hostname: node-1  # 只作用于 10-20-1-240 这个节点
      numVfs: 8 # 渴望的 VFs 数量
      resourceName: sriov_netdevice
    EOF
    ```
   
    >  下发后, 因为需要配置节点启用 SR-IOV 功能，可能会重启节点。如有需要，指定工作节点而非 Master 节点。
    >  resourceName 不能为特殊字符，支持的字符: [0-9],[a-zA-Z] 和 "_"。

    在下发 SriovNetworkNodePolicy CRs 之后，再次查看 SriovNetworkNodeState CRs 的状态, 可以看见 status 中 VFs 已经得到配置:

    ```shell
    $ kubectl get sriovnetworknodestates.sriovnetwork.openshift.io -n sriov-network-operator node-1 -o yaml
    ...
    - Vfs:
        - deviceID: 1018
          driver: mlx5_core
          pciAddress: 0000:04:00.4
          vendor: "15b3"
        - deviceID: 1018
          driver: mlx5_core
          pciAddress: 0000:04:00.5
          vendor: "15b3"
        - deviceID: 1018
          driver: mlx5_core
          pciAddress: 0000:04:00.6
          vendor: "15b3"
        deviceID: "1017"
        driver: mlx5_core
        mtu: 1500
        numVfs: 8
        pciAddress: 0000:04:00.0
        totalvfs: 8
        vendor: "8086"
    ...
    ```

    查看 Node 发现名为 `spidernet.io/sriov_netdevice` 的 sriov 资源已经生效，其中 VF 的数量为 8:

    ```shell
    ~# kubectl get  node  node-1 -o json |jq '.status.allocatable'
    {
      "cpu": "24",
      "ephemeral-storage": "94580335255",
      "hugepages-1Gi": "0",
      "hugepages-2Mi": "0",
      "spidernet.io/sriov_netdevice": "8",
      "memory": "16247944Ki",
      "pods": "110"
    }
    ```

## 安装 Spiderpool

1. 安装 Spiderpool。

    ```shell
    helm repo add spiderpool https://spidernet-io.github.io/spiderpool
    helm repo update spiderpool
    helm install spiderpool spiderpool/spiderpool --namespace kube-system
    ```

    > 如果您是国内用户，可以指定参数 `--set global.imageRegistryOverride=ghcr.m.daocloud.io` 避免 Spiderpool 的镜像拉取失败。

2. 创建 SpiderIPPool 实例。

    Pod 会从该子网中获取 IP，进行 Underlay 的网络通讯，所以该子网需要与接入的 Underlay 子网对应。
    以下是创建相关的 SpiderIPPool 示例

    ```shell
    cat <<EOF | kubectl apply -f -
    apiVersion: spiderpool.spidernet.io/v2beta1
    kind: SpiderIPPool
    metadata:
      name: ippool-test
    spec:
      default: true
      ips:
      - "10.20.168.190-10.20.168.199"
      subnet: 10.20.0.0/16
      gateway: 10.20.0.1
      multusName: kube-system/sriov-test
    EOF
    ```

3. 创建 SpiderMultusConfig 实例。

    ```shell
    $ cat <<EOF | kubectl apply -f -
    apiVersion: spiderpool.spidernet.io/v2beta1
    kind: SpiderMultusConfig
    metadata:
      name: sriov-test
      namespace: kube-system
    spec:
      cniType: sriov
      sriov:
        resourceName: spidernet.io/sriov_netdevice
    EOF
    ```
   
    > 注意: SpiderIPPool.Spec.multusName: `kube-system/sriov-test` 要和创建的 SpiderMultusConfig 实例的 Name 和 Namespace 相匹配
    > resourceName:  spidernet.io/sriov_netdevice 由安装 sriov-operator 指定的 resourcePrefix: spidernet.io 和创建 SriovNetworkNodePolicy CR 时指定的 resourceName: sriov_netdevice 拼接而成 

## 创建应用

1. 使用如下命令创建测试 Pod 和 Service：

    ```shell
    cat <<EOF | kubectl create -f -
    apiVersion: apps/v1
    kind: Deployment
    metadata:
      name: sriov-deploy
    spec:
      replicas: 2
      selector:
        matchLabels:
          app: sriov-deploy
      template:
        metadata:
          annotations:
            v1.multus-cni.io/default-network: kube-system/sriov-test
          labels:
            app: sriov-deploy
        spec:
          containers:
          - name: sriov-deploy
            image: nginx
            imagePullPolicy: IfNotPresent
            ports:
            - name: http
              containerPort: 80
              protocol: TCP
            resources:
              requests:
                spidernet.io/sriov_netdevice: '1' 
              limits:
                spidernet.io/sriov_netdevice: '1'  
    ---
    apiVersion: v1
    kind: Service
    metadata:
      name: sriov-deploy-svc
      labels:
        app: sriov-deploy
    spec:
      type: ClusterIP
      ports:
        - port: 80
          protocol: TCP
          targetPort: 80
      selector:
        app: sriov-deploy 
    EOF
    ```

    必要参数说明：

    > `spidernet/sriov_netdevice`: 该参数表示使用 Sriov 资源。
    >
    > `v1.multus-cni.io/default-network`：该 annotation 指定了使用的 Multus 的 CNI 配置。
    >
    > 更多 Multus 注解使用请参考 [Multus 注解](https://github.com/k8snetworkplumbingwg/multus-cni/blob/master/docs/quickstart.md)

2. 查看 Pod 运行状态

    ```shell
    ~# kubectl get pod -l app=sriov-deploy -owide
    NAME                           READY   STATUS    RESTARTS   AGE     IP              NODE        NOMINATED NODE   READINESS GATES
    sriov-deploy-9b4b9f6d9-mmpsm   1/1     Running   0          6m54s   10.20.168.191   worker-12   <none>           <none>
    sriov-deploy-9b4b9f6d9-xfsvj   1/1     Running   0          6m54s   10.20.168.190   master-11   <none>           <none>
    ```

3. 应用的 IP 将会自动固定在该 IP 范围内:

    ```shell
    ~# kubectl get spiderippool
    NAME         VERSION   SUBNET         ALLOCATED-IP-COUNT   TOTAL-IP-COUNT   DEFAULT   DISABLE
    ippool-test  4         10.20.0.0/16   2                    10               true      false
   
    ~#  kubectl get spiderendpoints
    NAME                           INTERFACE   IPV4POOL      IPV4               IPV6POOL   IPV6   NODE
    sriov-deploy-9b4b9f6d9-mmpsm   eth0        ippool-test   10.20.168.191/16                     worker-12
    sriov-deploy-9b4b9f6d9-xfsvj   eth0        ippool-test   10.20.168.190/16                     master-11
    ```

4. 测试 Pod 与 Pod 的通讯

    ```shell
    ~# kubectl exec -it sriov-deploy-9b4b9f6d9-mmpsm -- ping 10.20.168.190 -c 3
    PING 10.20.168.190 (10.20.168.190) 56(84) bytes of data.
    64 bytes from 10.20.168.190: icmp_seq=1 ttl=64 time=0.162 ms
    64 bytes from 10.20.168.190: icmp_seq=2 ttl=64 time=0.138 ms
    64 bytes from 10.20.168.190: icmp_seq=3 ttl=64 time=0.191 ms
   
    --- 10.20.168.190 ping statistics ---
    3 packets transmitted, 3 received, 0% packet loss, time 2051ms
    rtt min/avg/max/mdev = 0.138/0.163/0.191/0.021 ms
    ```

5. 测试 Pod 与 Service 通讯

    * 查看 Service 的 IP：

        ```shell
        ~# kubectl get svc
        NAME               TYPE        CLUSTER-IP     EXTERNAL-IP   PORT(S)              AGE
        kubernetes         ClusterIP   10.43.0.1      <none>        443/TCP              23d
        sriov-deploy-svc   ClusterIP   10.43.54.100   <none>        80/TCP               20m
        ```

    * Pod 内访问自身的 Service ：

        ```shell
        ~# kubectl exec -it sriov-deploy-9b4b9f6d9-mmpsm -- curl 10.43.54.100 -I
        HTTP/1.1 200 OK
        Server: nginx/1.23.3
        Date: Mon, 27 Mar 2023 08:22:39 GMT
        Content-Type: text/html
        Content-Length: 615
        Last-Modified: Tue, 13 Dec 2022 15:53:53 GMT
        Connection: keep-alive
        ETag: "6398a011-267"
        Accept-Ranges: bytes
        ```
