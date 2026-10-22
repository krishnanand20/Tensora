# syntax=docker/dockerfile:1
# JupyterLab + PyTorch + CUDA for notebook workloads (catalog id: pytorch-notebook).
# Runs as uid 1000 to match the pod securityContext in controlplane/internal/cluster/manifests.
# Pin the base image by digest before publishing.
FROM pytorch/pytorch:2.7.1-cuda12.8-cudnn9-runtime

RUN pip install --no-cache-dir "jupyterlab>=4.4,<4.5" ipywidgets \
 && useradd --create-home --uid 1000 --shell /bin/bash jovyan \
 && mkdir -p /home/jovyan/work && chown -R 1000:1000 /home/jovyan

USER 1000
WORKDIR /home/jovyan/work
EXPOSE 8888

# Auth: JUPYTER_TOKEN is injected per workload (Phase 3); until then the Gateway must front it.
CMD ["jupyter", "lab", "--ip=0.0.0.0", "--port=8888", "--no-browser", "--ServerApp.root_dir=/home/jovyan/work"]
