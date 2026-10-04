---
name: seusc-cluster
description: 通过 SEU SC Bridge 的 ssh seusc、SFTP/scp 和 Slurm 操作东南大学超算集群。用于检查资源、上传代码和数据、准备环境、提交或排查 CPU/GPU 作业及取回结果；也适用于 CLI agent 执行这些工作。仅查询或准备脚本时不实际提交作业。
---

# SEUSC 集群操作

使用本机已有的 `ssh seusc` 别名连接 SEU 集群。先识别当前账户和桥接版本，再完成用户要求的上传、运行、诊断或结果回收。本 skill 不依赖浏览器自动化、MCP 或某一种 agent CLI，只需要本地 shell、文件操作和 OpenSSH。

## 入口与事实来源

首次使用或连接状态变化后，用一个短命令检查：

```powershell
ssh -o BatchMode=yes -o ConnectTimeout=15 seusc 'hostname; whoami; pwd; type sbatch; sinfo --version; squeue -u $(id -un)'
```

- `seusc` 是本机 SSH 别名，SSH 配置负责端口和密钥；不要写死回环端口、用户名、Account 或家目录。SSH 登录别名中的 user 不一定是集群用户名。
- 连接失败时查 `seusc status` / `seusc doctor`。Windows 若未加入 PATH，可用 `& "$env:LOCALAPPDATA\Programs\SEUSC\seusc.exe" status`；先确认程序存在。验证码或失效登录交给用户在 SEU SC Bridge 中完成，不读取凭据、Cookie 或浏览器 profile。
- 当前状态以远端查询和实际报错为准；分区数、空闲情况、QoS、软件版本不可沿用旧快照。官方文档：<https://sc.seu.edu.cn/docs/>。站点可能需要校园网/VPN，可通过本机网络读取公开文档。
- 操作细节、本地 PowerShell/POSIX 命令、故障处理见 [使用文档](references/workflow.md)。首次传输、提交或诊断相关问题时读取对应章节。

## 传输与远端执行的关键约束

新版桥接支持 `sftp seusc`、`sftp -b batch.txt seusc` 和使用 SFTP 协议的现代 `scp`，包括二进制文件。旧客户端不一定具备这些功能；SFTP 失败时先确认本机安装版本，不要依据旧文档断定整个集群不支持文件传输。

- 小文件用 scp/SFTP。多文件项目通常先打包再传输；只打包此次任务需要的文件。查看退出码，重要输入/结果比较本地和远端 SHA-256。
- 不使用 `scp -O`、`scp -p`、SFTP 追加/续传、权限或时间戳保留、链接创建。完整 POSIX 文件语义不受支持；若需要执行权限，在远端用 `chmod`，也可直接 `bash script.sh`。
- 传输会在本机暂存完整文件，要有足够空间；大文件最后合并可能停在 100%，需等命令成功返回。下载运行中的日志可能遇到文件变化，可通过短 SSH 命令 `tail -n 80` 读取。
- SSH exec 无 stdin 管道，stdout/stderr 合流，输出可能含 ANSI/CRLF，命令长度约 3 KB。不要使用 `Get-Content ... | ssh ...`、`cat ... | ssh ...`、rsync、VS Code Remote SSH 或 SSH 隧道。
- 在本地生成 UTF-8 无 BOM、LF 行尾的 `.sh`/`.slurm`，上传后用短命令执行；避免用长内联脚本、Base64 命令或手工转义堆叠来传输文件。
- PowerShell 中远端 Bash 命令用单引号包住，避免本地展开 `$USER`、`$(...)`。对动态路径按各自 shell 正确引用；`JSON.stringify` 不是 shell 转义。

## Slurm 约束（2026-09-21/22 实测）

- 登录节点用于准备和管理；计算通过 `sbatch` 或 `srun` 分配节点。
- 必须显式选择分区和合理的时限。默认 `normal_test` 最长 30 分钟，平台另提示每隔 30 分钟清理任务，只用于快速验证。
- `sbatch --wrap` 被禁用，提交脚本文件。
- `--mem` 被拒绝，内存根据申请的 CPU 自动配给。不要尝试换成其他内存参数规避；高内存任务应检查分区 `DefMemPerCPU`，选择核数/大内存分区。
- `gpu_v100` 每张 GPU 自动配 3 核 CPU；`gpuB` 每张自动配 12 核。用 `--gres=gpu:N`，不要覆盖 `CUDA_VISIBLE_DEVICES`。`gpuB` 的网络限制见使用文档。
- `sbatch` 通常为 `/usr/bin/sbatch_wrapper` 的 alias，包含组配额检查。交互式 SSH 中用 `sbatch`；嵌套非交互 Bash 时 alias 未必展开，先查询并使用平台 wrapper，不要改为裸 `/usr/bin/sbatch` 来绕过检查。
- 每个作业脚本显式加载软件/激活环境，不依赖登录 shell 中已激活的 Conda。用 `module -t avail` 验证名称，旧样例可能引用已下线模块。
- 单进程多线程用 `--ntasks=1 --cpus-per-task=N`；MPI/DDP 要按程序实际启动方式设计，不能把申请多个 task 当成程序自动并行。

## 完成任务的流程

1. 按任务检查 `sinfo`、自己的 `squeue`、必要的 `scontrol show partition`、Account/QoS 和 `module`。若只是查看/写文档，到此及本地准备即可；不要为了“验证可用”实际跑计算。
2. 选择本次任务独立的远端工作目录，记录源文件版本、输入位置和资源参数，上传必要文件。已有授权足够时直接完成正常准备和提交，无需重复确认。
3. 选用 [最小验证脚本](assets/smoke.slurm)、[CPU 单进程模板](assets/cpu.slurm) 或 [单 GPU 模板](assets/gpu.slurm)。先修改资源与环境以匹配用户任务；模板不是性能最优配置。
4. 在实际提交目录执行 `bash -n job.slurm` 和平台 wrapper 的 `--test-only job.slurm`。这检查 shell 语法/调度参数，不执行脚本体，不证明依赖、数据、运行时或计费状态一定无误。dry-run 输出中的 `Job N to start ...` 不是已提交确认，也不是可靠的开始时间承诺。
5. 用户要求运行时，通过平台 wrapper **提交一次**，保存原始返回、确认的 Job ID、作业名、提交时间、工作目录和日志路径。输出可能附带排队序号，不假定整段输出是纯数字。使用本次独有作业名便于查重。
6. 若提交连接超时/断开，先用自己的队列、近期 `sacct`，结合名称、时间和工作目录核对是否已创建作业。无法判定时报告不确定状态，不盲目重新提交。
7. 按任务需要查看 `sjob` / `squeue` / `sacct` 和有限条日志；不要无休止高频轮询。被要求持续监控时使用宿主已有监控能力，否则明确最后观察到的状态。
8. 作业结束后核对 `State`、`ExitCode`、日志和预期产物，再下载相关结果。队列中消失不等于成功，dry-run 成功也不等于计算完成。失败重试前先修复原因；取消/清理仅针对用户要求或本任务创建的明确对象。

交付时简述实际完成了什么、使用的资源、Job ID/状态、结果位置和未验证项。不要把“写好了脚本”“通过 dry-run”“提交成功”“运行完成”混为一谈。
