#!/bin/bash
#SBATCH --job-name=seusc-cpu
#SBATCH --partition=6240-36C-192G
#SBATCH --nodes=1
#SBATCH --ntasks=1
#SBATCH --cpus-per-task=4
#SBATCH --time=01:00:00
#SBATCH --output=%x-%j.out
#SBATCH --error=%x-%j.err

set -e
cd "$SLURM_SUBMIT_DIR"
module load anaconda3-2024.10-1
if [ -n "${SEUSC_CONDA_ENV:-}" ]; then
    source "$(conda info --base)/etc/profile.d/conda.sh"
    conda activate "$SEUSC_CONDA_ENV"
fi
export OMP_NUM_THREADS="$SLURM_CPUS_PER_TASK"
export MKL_NUM_THREADS="$SLURM_CPUS_PER_TASK"
export OPENBLAS_NUM_THREADS="$SLURM_CPUS_PER_TASK"
export PYTHONUNBUFFERED=1
if [ "$#" -eq 0 ]; then
    echo 'Provide a command: sbatch cpu.slurm python -u main.py' >&2
    exit 2
fi
"$@"
