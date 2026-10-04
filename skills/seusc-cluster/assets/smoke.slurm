#!/bin/bash
#SBATCH --job-name=seusc-smoke
#SBATCH --partition=normal_test
#SBATCH --nodes=1
#SBATCH --ntasks=1
#SBATCH --cpus-per-task=1
#SBATCH --time=00:02:00
#SBATCH --output=%x-%j.out
#SBATCH --error=%x-%j.err

set -e
cd "$SLURM_SUBMIT_DIR"
printf 'job=%s node=%s cpus=%s\n' "$SLURM_JOB_ID" "$(hostname)" "$SLURM_CPUS_PER_TASK"
date -Is
