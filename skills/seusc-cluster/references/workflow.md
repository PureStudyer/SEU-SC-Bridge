# SEUSC 集群完整操作参考

用于 CLI agent 在本地终端通过 SEU SC Bridge 管理集群。PowerShell/POSIX 示例在本机执行；标为“集群内”的 Bash 示例在 SSH 会话中执行。读取 SKILL.md 后按以下章节处理任务。

## 1. 连接与账户

本机需安装并登录 SEU SC Bridge，并具有校园网/VPN访问条件。OpenSSH 提供 ssh/scp/sftp。先查：

```powershell
ssh -G seusc
ssh -o BatchMode=yes -o ConnectTimeout=15 seusc 'hostname; whoami; pwd; type sbatch; sinfo --version'
ssh seusc 'sacctmgr -nP show assoc where user=$(id -un) format=Cluster,Account,User,Partition,QOS,DefaultQOS'
ssh seusc 'squeue -u $(id -un)'
```

`ssh -G` 的 user 是桥接用户，不能代替集群 `whoami`。不写死以前账户的家目录或 Slurm Account。多账户时选择用户要求的账户，并在提交参数指定 `--account=实际账户`。查询不到关联或限制时报告可见性问题，不将空值解释成无限制。

连接问题使用 `seusc status`、`seusc doctor`。Windows 未加入 PATH 时通常可用：

```powershell
& "$env:LOCALAPPDATA\Programs\SEUSC\seusc.exe" status
```

macOS 通常为 `/Applications/SEU SC Bridge.app/Contents/MacOS/seusc`。先确认安装路径。验证码/失效登录由用户在应用中处理，不读取密码、Cookie、Bearer 或浏览器 profile。重启会影响活动会话，仅在故障需要时使用；例行检查不 logout、不删除状态、不覆盖 SSH 配置。

用户只要求查询、准备脚本或写文档时不实际启动计算。用户要求运行明确任务时，完成必要的准备、传输、提交和结果检查，无需反复确认；资源规模不清楚且影响成本/可行性时先检查程序和输入，再询问关键缺失信息。

## 2. 资源选择

```powershell
ssh seusc 'sinfo -o "%P %a %l %D %c %m %G"'
ssh seusc 'sinfo -o "%P %D %t %C"'
ssh seusc 'scontrol show partition 6240-36C-192G'
```

`%C` 是 allocated/idle/other/total CPU。节点数量包括不可用节点；CPU 空闲不代表 GPU 空闲。需要 GPU 详情时用 `sinfo -N -p gpu_v100` 选出实际节点名，再 `scontrol show node 节点名` 查 GRES 配置和已分配资源。在已分配 GPU 作业内用 `nvidia-smi` 确认实际设备。

按账户查询出的 QoS 检查限制，例如集群内：

```bash
# 将 ogsp 替换为自己的实际 QoS
sacctmgr -nP show qos where name=ogsp format=Name,MaxWall,MaxTRESPerJob,MaxTRESPerUser,MaxJobsPerUser,MaxSubmitJobsPerUser
```

参考分区（2026-09-22 查询，不固定节点数或长期可用性）：

| 需求 | 分区候选 | 注意 |
| --- | --- | --- |
| 快速验证 | normal_test | 默认分区、最长30分钟；平台另提示定期清理任务 |
| 常规 CPU | 6240-36C-192G、6126-24C-192G、2680v4-28C-256G | 根据核数、内存和排队选择 |
| 海光 CPU | 7495-128C-384G | 检查二进制/库适配；部分模块有 hygon 版本 |
| 大内存 | 7542-64C-512G、6126-24C-768G、9462-64C-1024G | 查看实际 DefMemPerCPU |
| V100 GPU | gpu_v100 | 每 GPU 自动分配3核CPU |
| 大显存 GPU | gpuB | 每 GPU 自动分配12核CPU；官方描述为单机任务 |

实际型号、内存和显存不能仅由分区名字推断。官方文档描述 V100 每卡32G显存、gpuB 每卡80G；当前 Slurm GRES 只显示通用 gpu，不能据此认定 gpuB 的具体型号。

`--mem` 被拒绝，内存跟随 CPU 核数自动配置。高内存任务依据 `DefMemPerCPU` 估算核数或选择大内存分区，不把整机内存当作每个作业额度，也不改用其他内存选项绕过政策。不要无需求地 `--exclusive` 占整节点。

官方说明 gpuB 节点无法访问外网，其他节点也受白名单/校园网络条件限制。代码、包、数据和模型提前准备到共享存储；只针对任务所需地址验证连通性，不假设 GitHub/Hugging Face 等一直可达。

## 3. 存储与目录

```powershell
ssh seusc 'pwd; df -h "$HOME"; ls -ld /seu_share /seu_share2 /seu_nvme'
```

家目录以 pwd 为准，不能据旧用户名拼接。其他存储不一定已有当前用户可写目录。`df` 表示整个文件系统空间，不是组/个人剩余额度。官方默认政策为 share 组免费2T、nvme组免费500G，实际配额需在网页“团队管理 → 配额管理”核实。曾在 login01 上遇到 mmlsquota 报 GPFS daemon 不可用，这不证明共享存储不可访问，不尝试管理或修复集群文件系统。

每次运行使用独立工作目录，例如 `~/jobs/<project>/<run-id>`。环境和大型只读输入可复用，不必每次复制。记录源码版本/哈希、输入、资源、目录、提交时间、Job ID及结果路径。示例选择一个尚未存在的 run-id：

```powershell
ssh seusc 'mkdir -p ~/jobs/demo && mkdir ~/jobs/demo/run-001'
```

第二个 mkdir 不带 -p，避免静默复用已有目录。用户要求继续现有任务时可复用其目录，不强制新建。

## 4. 文件传输和完整性

### 单文件

```powershell
scp .\train.py seusc:~/jobs/demo/run-001/train.py
scp .\job.slurm seusc:~/jobs/demo/run-001/job.slurm
scp seusc:~/jobs/demo/run-001/result.json .\downloads\result.json
```

先确保源文件和目标父目录存在；本地下载目录要先创建。scp 可覆盖同名文件，选择独立结果目录或确认覆盖属于当前任务。现代 scp 默认使用 SFTP，不加 `-O` 或 `-p`。

### 多文件项目

示例文件必须存在，只打包本次任务需要的输入：

```powershell
tar -czf payload.tar.gz train.py requirements.txt configs
tar -tzf payload.tar.gz
scp .\payload.tar.gz seusc:~/jobs/demo/run-001/payload.tar.gz
Get-FileHash -Algorithm SHA256 .\payload.tar.gz
ssh seusc 'cd ~/jobs/demo/run-001 && sha256sum payload.tar.gz'
# 比较哈希后再解压
ssh seusc 'cd ~/jobs/demo/run-001 && tar -xzf payload.tar.gz'
```

不默认打包 .git、凭据、登录状态、无关数据和本地虚拟环境。外来归档先检查绝对路径、..、链接等条目，在独立目录解压。Windows Conda/venv通常不能直接复制到Linux运行。

目录传输优先归档，不把 scp -r 当作完整权限、链接和文件系统镜像。需要执行位/链接时可在合适系统生成 tar，再远端解包；SFTP本身不提供完整POSIX语义。大型归档的高负载压缩也应申请计算资源。

### SFTP批处理

本机创建 UTF-8无BOM的 transfer.sftp，Windows本地路径用正斜杠，空格路径加引号。例如上传阶段：

```text
put "D:/work/train.py" "jobs/demo/run-001/train.py"
bye
```

```powershell
sftp -b .\transfer.sftp seusc
if ($LASTEXITCODE -ne 0) { throw 'SFTP failed' }
```

结果阶段另建包含 `get "jobs/demo/run-001/result.json" "D:/work/downloads/result.json"` 的批处理。支持 put/get/ls/mkdir/rename/rm/rmdir；不使用追加、续传、chmod、保留权限/时间戳、链接创建。关键命令不加忽略失败前缀。

文件在桥接本机暂存，预留与文件相当的空间；100%后可能仍在远端合并/改名，等待关闭确认与成功退出码。内置下载大小校验不是内容哈希验证，关键输入/结果要比 SHA-256。正在写的日志可能因变化导致下载失败，可先 SSH tail 或等任务结束。

## 5. 编码、命令和退出码

远端命令外用单引号，避免 PowerShell 展开 Bash 变量：

```powershell
ssh seusc 'printf "%s\n" "$HOME"; squeue -u $(id -un)'
```

动态路径按对应 shell 转义；JSON.stringify不是shell转义。复杂逻辑生成本地脚本再上传，避免约3KB的exec长度限制。stdin管道不可用，不用 `Get-Content | ssh`、`cat | ssh`、`ssh ... < script.sh` 或 rsync。远端命令内部使用管道则不受此限制。

shell/slurm 文件用UTF-8无BOM、LF。PowerShell5.1默认编码可能不符，写文件时指定：

```powershell
# $content 是已准备好的脚本文本，$path 是本地绝对输出路径
$utf8 = New-Object System.Text.UTF8Encoding($false)
[IO.File]::WriteAllText($path, $content.Replace("`r`n", "`n"), $utf8)
```

`sbatch job.slurm` 或 `bash run.sh` 不需要执行位；必须直接运行时，远端对该文件 chmod。macOS/Linux本机同样用scp上传脚本和短ssh命令调用即可。

桥接返回远端命令退出码，但stdout/stderr合流且可能带ANSI/CRLF。解析前清理显示控制字符，同时保留原始返回。多步骤依赖用 `&&` 或脚本正确传播失败；不能仅看最后一个无关查询成功就认为前面提交成功。

## 6. 软件和环境

```powershell
ssh seusc 'MODULES_PAGER=cat module -t avail 2>&1'
ssh seusc 'MODULES_PAGER=cat module show anaconda3-2024.10-1 2>&1'
ssh seusc 'ls /seu_share/home/examples'
```

现有模块包括Anaconda、CUDA、GCC、OpenMPI、Intel/oneAPI、Singularity及科研软件；名称/版本要现查。官方python_cuda.sh曾引用已下线模块，不原样照搬。科研软件优先参考其对应样例的模块组合和MPI启动方式。

### Python环境（集群内）

优先查找已有适合环境，新建时遵循项目Python/依赖版本。下面demo-env、Python3.11均为例子：

```bash
module load anaconda3-2024.10-1
source "$(conda info --base)/etc/profile.d/conda.sh"
conda env list
conda create -n demo-env python=3.11 -y
conda activate demo-env
python -m pip install -r ~/jobs/demo/run-001/requirements.txt
```

检查 conda envs_dirs/pkgs_dirs 位于用户可写共享存储；不要修改共享base环境。编译大量扩展、需要GPU检测的安装应在分配资源后执行。不要默认修改.bashrc；普通CLI流程在作业脚本中初始化即可，网页环境选择器有需要时才按官方说明配置。

CPU/GPU模板支持 `SEUSC_CONDA_ENV` 环境名或路径；未设置时使用模块默认Python，不保证有项目依赖。作业中显式激活环境，因为sbatch_wrapper可能清理提交进程继承的Conda状态。安装PyTorch等遵循项目锁定版本、驱动和设备兼容性，不盲目组合CUDA/GCC模块。

自己安装软件用用户共享目录，如 `cmake -DCMAKE_INSTALL_PREFIX=...` 或 `./configure --prefix=...`；无root权限，不执行sudo/yum/apt安装系统包。编译密集任务通过Slurm运行，之后显式设置PATH/库路径。

### 容器

平台网页有Enroot镜像/模型开发入口；命令行可查Singularity模块，二者格式不同，不把Enroot镜像当作.sif。以下是已分配GPU的作业脚本示例，模块/镜像须先确认：

```bash
module load singularity-4.1.2
singularity exec --nv --bind "$PWD:/work" project.sif python /work/train.py
```

不假定root Docker daemon可用。网页实例的SSH/Jupyter入口与本地桥接不同，不能据此使用 `ssh -L` 或VS Code Remote SSH连接seusc。容器/MPI/深度学习环境仍需任务级运行验证。

## 7. 脚本、dry-run与提交

本skill提供 `assets/smoke.slurm`（1CPU/2分钟验证）、`assets/cpu.slurm`（单进程4CPU）、`assets/gpu.slurm`（1V100/3CPU）。后两者接收命令argv，支持可选 `SEUSC_CONDA_ENV`。它们不写死Account、不指定内存、不发邮件。

复制模板为job.slurm，修改分区、资源、时间和唯一名称。`#SBATCH`应在第一个实际shell命令之前；指令里的$变量不会按普通shell展开，动态值用sbatch命令选项或预先生成具体内容。

本机示例：

```powershell
scp .\job.slurm seusc:~/jobs/demo/run-001/job.slurm
ssh seusc 'cd ~/jobs/demo/run-001 && bash -n job.slurm'
ssh seusc 'cd ~/jobs/demo/run-001 && /usr/bin/sbatch_wrapper --test-only --job-name=demo-run-001 job.slurm python -u train.py'
```

wrapper路径以 `type sbatch` 为准。不要用裸/usr/bin/sbatch绕过配额检查。test-only只验证调度参数，不运行脚本体，不验证依赖、数据、程序结果或保证计费状态。其 `Job N to start ...` 不是已提交作业，不是可靠的开始时间承诺。

用户要求运行后，提交一次：

```powershell
ssh seusc 'cd ~/jobs/demo/run-001 && SEUSC_CONDA_ENV=demo-env /usr/bin/sbatch_wrapper --job-name=demo-run-001 job.slurm python -u train.py'
```

保存完整返回，寻找 `Submitted batch job N` 并查询该ID。wrapper可能附带排队序号，不把整段输出当数字；--parsable也不能未经验证就假定纯数字返回。没有明确确认时按名称、时间、目录查重，不猜ID或盲目再次提交。

模板用 `"$@"` 执行argv。若程序需要管道/重定向/多步shell，另写run.sh，提交 `job.slurm bash run.sh`；不要把整段shell文本当成单个可执行参数。保留程序非零退出状态，不用 `|| true` 隐藏错误，自定义含管道脚本应正确处理管道失败。

## 8. 多核、批量、MPI和多GPU

- 单进程多线程：1 task、N cpus-per-task；模板设置OMP线程数，CPU模板另设置MKL/OpenBLAS，实际并行仍由程序实现。
- 独立批量任务：按需求设计array，先查支持和限制，以 `%并发数` 限流，不因输入多就自动申请大量资源。
- MPI：按对应软件样例选择MPI模块及mpirun/srun，申请实际task数量；只有程序支持跨节点时才增加nodes，运行库与编译库保持兼容。
- 单节点PyTorch DDP：2张V100可设计为 `--ntasks=1 --cpus-per-task=6 --gres=gpu:2`，由 `torchrun --standalone --nnodes=1 --nproc_per_node=2 train.py` 启动。程序必须支持DDP，这只是设计例子，本skill未实测训练。
- gpuB每卡12核，官方描述为单机任务。跨节点训练需单独核实网络、策略、程序支持与启动方式。

多进程/多节点不能直接照用单进程模板。新增资源配置先dry-run；不硬编码CUDA_VISIBLE_DEVICES、不手工挑选别人的GPU。

## 9. 交互调试

先在本机 `ssh seusc`，再在集群终端申请：

```bash
srun -p normal_test -N 1 -n 1 -c 1 -t 00:10:00 --pty /bin/bash
hostname
# 调试完毕释放allocation
exit
```

GPU用gpu_v100、1task、3CPU、--gres=gpu:1和短时限。agent须有交互PTY；无人值守任务优先sbatch。不直接ssh到计算节点占资源，不把登录节点作为训练或重计算节点。交互会话断开后先查自己的allocation，不能假定恢复原来的PTY。

## 10. 监控、恢复、取消与取回

```bash
# 集群内：JOBID替换为已确认的数字
squeue -u "$USER"
sjob -j JOBID
scontrol show job JOBID
sacct -j JOBID --format=JobID,JobName,State,ExitCode,Elapsed,AllocCPUS,MaxRSS -P
speek JOBID
speek -e JOBID
```

非交互环境没有shist alias时直接用sacct。查看MaxRSS时保留step记录；批作业行为空不代表内存使用为零。历史记账可能延迟或权限受限。

| 状态/现象 | 动作 |
| --- | --- |
| PENDING / Resources / Priority | 正常排队，检查请求和原因，不重复提交 |
| QOS/Account/Partition限制 | 查账户关联和策略，保留原报错 |
| 队列消失 | 查sacct与日志，不能当作成功 |
| COMPLETED + ExitCode 0:0 | 再核对预期结果文件及内容 |
| OUT_OF_MEMORY | 查step和内存配比，调整分区/核数/程序，不加--mem |
| TIMEOUT | 检查时限和测试分区清理；正式分区、checkpoint按需使用 |
| FAILED / 非零退出 | 查输入、依赖和错误日志，修复原因再重试 |

Python用-u或PYTHONUNBUFFERED避免输出缓冲。运行中日志用短SSH `tail -n 80 实际日志路径`，不启动无法结束的tail轮询。合理间隔查询；需要持续监控时用agent宿主可用机制，没有则说明最后观察到的状态，不承诺离开会话后自行通知。

Batch任务通常独立于SSH连接。若提交响应丢失，先查重：

```bash
squeue -u "$USER" -n demo-run-001
# 日期换为本次实际提交日期，必要时缩小窗口
sacct -S YYYY-MM-DD -u "$USER" --name=demo-run-001 --format=JobID,JobName,Submit,State,WorkDir -P
```

以唯一名称、时间和目录匹配。历史尚未出现不一定未提交，不能无依据重复提交；无法判断就报告不确定状态。

作业完成后下载已稳定产物和日志：

```powershell
scp seusc:~/jobs/demo/run-001/result.json .\downloads\result.json
scp seusc:~/jobs/demo/run-001/demo-run-001-JOBID.out .\downloads\
```

从scontrol/实际目录确认日志文件名，提前创建本地目录，关键产物验证SHA-256。用户要求或当前任务需要时仅 `scancel JOBID`；不用scancel -u取消其全部作业。清理仅针对明确属于本任务的文件，保留待诊断任务日志和输入。结束报告区分：已准备、dry-run通过、已提交、运行中、成功完成。

## 11. 故障速查

| 问题 | 处理 |
| --- | --- |
| --wrap被禁用 | 上传脚本文件提交 |
| --mem被拒绝 | 去掉，按CPU自动内存配比规划 |
| /bin/bash^M | LF、UTF-8无BOM重新上传 |
| Conda环境不存在 | 同一Anaconda模块，显式source conda.sh，查环境名/绝对路径 |
| CUDA不可用 | 在分配到的节点查设备/包，不覆盖CUDA_VISIBLE_DEVICES |
| SFTP subsystem不可用 | 确认运行中桥接已升级，不回退scp -O |
| 100%后等待 | 等上游合并关闭及成功退出码 |
| 磁盘满/配额超限 | 检查本机暂存与远端组配额，不只看df |
| 下载size mismatch | 文件仍在写，SSH tail或等结束 |
| 权限/时间戳设置失败 | 去除保留属性选项，必要chmod远端执行 |
| 欠费/组配额报错 | 平台核实，不绕过wrapper |
| bashrc造成协议异常 | 避免无条件输出，不覆盖用户现有配置 |

## 12. 来源与验证边界

- 2026-09-22检查了本项目README、docs/guide.md及internal/sftpserver/server.go。SFTP通过Finder网页接口，SSH命令通过WebShell PTY。
- 2026-09-21/22查询login01与Slurm23.11.10，验证了账户查询、分区与模块发现，以及禁用参数/GPU核数配比的dry-run返回。后续以现场查询为准。
- [官方总说明](https://sc.seu.edu.cn/docs/)
- [CPU、Slurm、存储](https://sc.seu.edu.cn/docs/hpc/contents.html)
- [GPU、环境、容器](https://sc.seu.edu.cn/docs/ai/contents.html)
- [软件与样例](https://sc.seu.edu.cn/docs/software/contents.html)
- [常见问题](https://sc.seu.edu.cn/docs/faq/contents.html)
- [费用管理](https://sc.seu.edu.cn/docs/fee/contents.html)

完整工作流不等于所有科研软件、容器、多节点训练或费用策略已实测。具体任务仍需验证运行时与预期结果；不将文档设计示例标成已经执行成功。

### 本skill交付时的验证（2026-09-22）

使用当前运行中的桥接完成了4KiB随机二进制的scp上传、SFTP批量下载，本地原件、下载件及远端SHA-256一致。三个模板均通过远端bash -n与平台sbatch_wrapper --test-only。测试使用独立目录，文件已通过SFTP逐项删除并移除空目录；没有实际提交计算作业。

GPU dry-run曾给出异常遥远的预计时间，说明预计开始时间不能当作可靠承诺；本次只把返回用作调度请求校验，不声称GPU已分配或程序已运行。
