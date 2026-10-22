# ubuntu-24.04-cuda (VM base image)

Catalog id `ubuntu-24.04-cuda` refers to the CDI **DataSource** of the same name in the
`tensora-images` namespace. Customer VMs clone their root disk from it.

## Phase 1: stock cloud image

`datasource.yaml` imports the Ubuntu 24.04 cloud image. The NVIDIA driver is **not**
included, so customers install it themselves (`sudo ubuntu-drivers install --gpgpu`).
That's fine for internal testing but not for customers.

```bash
kubectl apply -f images/vm/ubuntu-24.04-cuda/datasource.yaml
kubectl -n tensora-images get dv ubuntu-24.04-cuda -w     # wait for Succeeded
```

## Phase 3: prebaked image

Bake the driver, CUDA, and qemu-guest-agent into the qcow2 so VMs are usable on first boot:

1. Start from a dated Ubuntu cloud image. Pin the date, never `current`.
2. Use `virt-customize` (libguestfs) or Packer to install `nvidia-driver-<branch>-server`,
   `cuda-toolkit`, and `qemu-guest-agent`, then run `cloud-init clean`.
3. Upload it to the object store and point `datasource.yaml` at it (with an `http` or `s3` source).
4. Version the DataSource name (`ubuntu-24.04-cuda-2026-11`) and update `catalog.yaml`.
   Existing VMs keep their own disks.
