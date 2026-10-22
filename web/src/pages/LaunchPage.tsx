import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type FormEvent, useState } from "react";
import { useNavigate } from "react-router";

import { api } from "../api/client";
import type { CreateWorkload, WorkloadKind } from "../api/types";

const kinds: { value: WorkloadKind; label: string }[] = [
  { value: "endpoint", label: "Model endpoint" },
  { value: "notebook", label: "Notebook" },
  { value: "vm", label: "GPU VM" },
];

export function LaunchPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const catalog = useQuery({ queryKey: ["catalog"], queryFn: api.catalog });
  const [kind, setKind] = useState<WorkloadKind>("endpoint");
  const create = useMutation({
    mutationFn: api.createWorkload,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["workloads"] });
      navigate("/");
    },
  });

  if (catalog.isPending) return <p>Loading…</p>;
  if (catalog.isError) return <p className="error">{catalog.error.message}</p>;

  const images = catalog.data.images.filter((i) => i.kind === kind);
  const gpus = catalog.data.gpu_classes.filter((g) => kind !== "vm" || g.supports_vm);

  function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    const input: CreateWorkload = {
      name: String(f.get("name")),
      kind,
      image_id: String(f.get("image_id")),
      gpu_class: String(f.get("gpu_class")),
      gpu_count: Number(f.get("gpu_count")),
    };
    if (kind === "vm") input.ssh_public_key = String(f.get("ssh_public_key")).trim();
    if (kind === "endpoint") input.model_uri = String(f.get("model_uri")).trim();
    create.mutate(input);
  }

  return (
    <section>
      <h1>Launch</h1>
      <form className="form" onSubmit={onSubmit}>
        <label>
          Type
          <select value={kind} onChange={(e) => setKind(e.target.value as WorkloadKind)}>
            {kinds.map((k) => (
              <option key={k.value} value={k.value}>
                {k.label}
              </option>
            ))}
          </select>
        </label>
        <label>
          Name
          <input name="name" required pattern="[a-z0-9]([-a-z0-9]{0,38}[a-z0-9])?" placeholder="my-llm" />
        </label>
        <label>
          Image
          <select name="image_id" key={kind}>
            {images.map((i) => (
              <option key={i.id} value={i.id} title={i.description}>
                {i.display_name}
              </option>
            ))}
          </select>
        </label>
        <div className="row">
          <label>
            GPU
            <select name="gpu_class">
              {gpus.map((g) => (
                <option key={g.id} value={g.id}>
                  {g.display_name}
                </option>
              ))}
            </select>
          </label>
          <label>
            Count
            <input name="gpu_count" type="number" min={1} max={8} defaultValue={1} />
          </label>
        </div>
        {kind === "vm" && (
          <label>
            SSH public key
            <textarea name="ssh_public_key" required rows={3} placeholder="ssh-ed25519 AAAA… you@laptop" />
          </label>
        )}
        {kind === "endpoint" && (
          <label>
            Model (safetensors only)
            <input name="model_uri" required placeholder="hf://Qwen/Qwen2.5-0.5B-Instruct" />
          </label>
        )}
        {create.isError && <p className="error">{create.error.message}</p>}
        <button type="submit" disabled={create.isPending}>
          {create.isPending ? "Launching…" : "Launch"}
        </button>
      </form>
    </section>
  );
}
