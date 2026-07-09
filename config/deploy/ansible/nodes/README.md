# 定制 V2bX 节点一键部署(gRPC 接入面板)

用 Ansible 把定制版 V2bX 部署到落地服务器,让它通过 gRPC 接到新面板。渲染 config、推二进制、装 systemd、重启,一步到位。支持一台一台来(`-l`)。

## 节点 ID 规则

- `ID <= 100` 是 legacy 节点区间。当前数据库里的节点就是最后一批 legacy 节点,统一打 `legacy-node` 标签。
- `ID > 100` 预留给 V2bX 自注册节点。新增节点默认不再走手动创建/固定 NodeID 流程。
- 本目录的 `deploy_v2bx.yml` 是固定 `NodeID + ApiKey` 接管流程,只用于迁移、修复或重新部署 legacy 节点。
- 新节点应使用 `AutoRegister=true + AuthKey` 流程,由面板分配 `node_id/api_key/secret`,节点本地保存 credential。

## 前置

1. **准备定制 V2bX 二进制**(按目标机架构):
   ```bash
   mkdir -p /home/dev/anixops/V2bX_AnixOps/build/inventory
   # 从 GitHub Actions 下载对应架构的 V2bX artifact 后放到:
   # /home/dev/anixops/V2bX_AnixOps/build/inventory/V2bX_linux_amd64
   # /home/dev/anixops/V2bX_AnixOps/build/inventory/V2bX_linux_arm64
   ```
   路径填进 `group_vars/all.yml` 的 `v2bx_binary_amd64_local` / `v2bx_binary_arm64_local`。playbook 会按 `v2bx_arch` 或远端架构自动选择。
   发行或生产部署不得在本机编译 V2bX。

2. **准备变量**:
   ```bash
   cd config/deploy/ansible/nodes
   cp group_vars/all.yml.example group_vars/all.yml
   # 编辑 all.yml: 填面板地址(HTTP 或 HTTPS 二选一)、二进制路径
   ```

3. **准备 inventory**:
   ```bash
   cp inventory.ini.example inventory.ini
   # legacy 节点每台一行, 标 node_id=<面板里该节点ID>, 配好 SSH
   # Oracle ARM / Ampere 节点建议加: v2bx_arch=arm64
   # 不写时 deploy_v2bx.yml 会从远端架构自动识别; deploy_from_inventory.sh 默认为 amd64。
   ```

4. **拿管理员 token**(用于自动拉 legacy 节点的 api_key):登录面板后台,从浏览器 devtools 或登录接口取 JWT。也可跳过自动拉取,在 inventory 里手填 `api_key=`。

## 用法

先干跑看渲染结果(不改任何东西):
```bash
cd config/deploy/ansible/nodes
export ANSIBLE_CONFIG=../ansible.cfg
ansible-playbook -i inventory.ini deploy_v2bx.yml -l kfc-jp \
  -e admin_token=<管理员JWT> --check
```

正式部署单台 legacy 节点(KFC JP):
```bash
export ANSIBLE_CONFIG=../ansible.cfg
ansible-playbook -i inventory.ini deploy_v2bx.yml -l kfc-jp \
  -e admin_token=<管理员JWT>
```

## 推荐:直接用自动脚本

如果你已经把节点写进 `inventory.ini`, 推荐直接用本目录的脚本:

```bash
cd config/deploy/ansible/nodes
./deploy_from_inventory.sh --host kfc-jp
./deploy_from_inventory.sh --hosts kfc-jp,hytron-hk-2
./deploy_from_inventory.sh --all
```

脚本会自动:

1. 读取 `inventory.ini` 的 `[v2bx_nodes]`
2. 优先使用 inventory 手填的 `api_key`, 没填时尝试从本地面板 SQLite 按 `node_id` 自动读取
3. 按每台主机的 `v2bx_arch` 识别需要 `amd64` 还是 `arm64` 二进制
4. 默认拒绝本地编译；生产/发行路径使用 `--skip-build` 和 GitHub Actions 产物
5. 生成临时 inventory, 自动注入对应架构的 `v2bx_binary_local`
6. 调用 `ansible-playbook deploy_v2bx.yml`

常用参数:

```bash
./deploy_from_inventory.sh --list
./deploy_from_inventory.sh --host akko-uk --check --skip-build
./deploy_from_inventory.sh --all --skip-build --admin-token <管理员JWT>
```

旧的本地编译路径只保留给开发或紧急人工操作，必须显式设置 `ALLOW_LOCAL_BUILD=1`，且不能作为发行构建来源。

### Oracle ARM 怎么标

在对应主机那一行直接加:

```ini
akko-uk ansible_host=185.217.110.160 ansible_port=22 ansible_user=root ansible_ssh_pass=... node_id=7 v2bx_arch=arm64
```

直接跑 `deploy_v2bx.yml` 时,不写 `v2bx_arch` 会从远端架构自动识别。使用 `deploy_from_inventory.sh` 时,脚本为了先编译本地二进制仍默认按 `amd64` 处理,ARM 节点建议显式写 `v2bx_arch=arm64`。

一台跑通后,去掉 `-l` 部署全部,或逐台 `-l <host>` 一个个来:
```bash
export ANSIBLE_CONFIG=../ansible.cfg
ansible-playbook -i inventory.ini deploy_v2bx.yml -e admin_token=<管理员JWT>
```

## 验证一个节点迁移成功

1. playbook 末尾 `Verify V2bX is active` 通过(服务 active),并打印最近日志。
2. 面板 `/admin/nodes`:该节点变"在线"(5 分钟内有心跳)。
3. 真机连接:客户端导入某用户订阅,选该节点,实测能上网。SS2022 节点会自动拿到面板下发的 server_key。

## 登录:首次密码 → 之后私钥

inventory 里每行的登录认证二选一(详见 `inventory.ini.example` 顶部注释):
- 首次:`ansible_ssh_pass=<密码>`(本机需装 `sshpass`)
- 之后:`ansible_ssh_private_key_file=<私钥路径>`

一键把公钥装到节点(用密码登录那次跑),之后即可切私钥免密:
```bash
# 用当前 inventory(此时还是密码登录)把你的公钥装上
export ANSIBLE_CONFIG=../ansible.cfg
ansible-playbook -i inventory.ini bootstrap_ssh_key.yml -l kfc-jp
# 默认装 ~/.ssh/id_ed25519.pub, 换公钥用 -e ssh_pubkey_file=/path/to/your.pub
```
装完把该行的 `ansible_ssh_pass=...` 改成 `ansible_ssh_private_key_file=~/.ssh/xxx.key`,
后续所有 playbook 都走私钥,不再需要 sshpass。私钥建议统一放一个目录(如 `~/.ssh/v2bx/`),每行指向对应文件。

## 说明

- **不使用官方 install.sh**:只推你编译好的定制二进制 + 装 systemd 服务,不会被官方版覆盖。
- **api_key 来源**:legacy 接管 playbook 的 pre_task 在 localhost 上调 `GET /api/v2/admin/nodes/:id/credentials`(带 admin_token)自动取;inventory 手填 `api_key=` 则优先用手填的。
- **HTTP / HTTPS**:`all.yml` 里 `grpc_use_tls` 控制。false→ 直连 IP:50051;true→ 域名:443 + `GRPCServerName`。
- **子节点(中转)**:legacy 接管时和普通节点一样填 node_id 即可,SS2022 的 server_key 由面板 gRPC 按父节点 created_at 自动算好下发,节点端无需特殊配置。
- `admin_token` 只通过 `-e` 传入,不写进任何文件。
