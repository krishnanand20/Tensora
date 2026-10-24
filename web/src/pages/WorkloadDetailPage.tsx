import { useMutation, useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router";

import { api } from "../api/client";
import { describeChange, isSettling } from "../workloads";

/**
 * One workload: its complete timeline, and a download of everything recorded about it (ADR-0015).
 * Deleted workloads open too, so their history is never out of reach.
 */
export function WorkloadDetailPage() {
  const { id = "" } = useParams();
  const workload = useQuery({
    queryKey: ["workload", id],
    queryFn: () => api.getWorkload(id, true),
    refetchInterval: (q) => (q.state.data && isSettling(q.state.data.state) ? 2000 : false),
  });
  const events = useQuery({
    queryKey: ["workload-events", id],
    queryFn: () => api.workloadEvents(id),
    refetchInterval: workload.data && isSettling(workload.data.state) ? 2000 : false,
  });
  const download = useMutation({
    mutationFn: () => api.exportWorkload(id),
    onSuccess: ({ blob, filename }) => {
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = filename;
      link.click();
      URL.revokeObjectURL(url);
    },
  });

  if (workload.isPending) return <p>Loading…</p>;
  if (workload.isError) return <p className="error">{workload.error.message}</p>;
  const w = workload.data;

  return (
    <section>
      <p>
        <Link to="/">← Workloads</Link>
      </p>
      <div className="section-head">
        <h1>{w.name}</h1>
        <button disabled={download.isPending} onClick={() => download.mutate()}>
          {download.isPending ? "Preparing…" : "Download full log"}
        </button>
      </div>
      {download.isError && <p className="error">{download.error.message}</p>}
      <p className="muted">
        {w.kind} · <span className={`state state-${w.state}`}>{w.state}</span> · {w.gpu_count} × {w.gpu_class} · id {w.id}
      </p>
      {w.state === "deleted" && (
        <p className="muted">This workload is deleted. Its timeline and full log stay available here.</p>
      )}

      <h2>Timeline</h2>
      <p className="muted">Every state change and note, when, and why. Nothing is filtered out.</p>
      {events.isError && <p className="error">{events.error.message}</p>}
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Time</th>
              <th>Change</th>
              <th>Reason</th>
            </tr>
          </thead>
          <tbody>
            {(events.data?.items ?? []).map((e) => (
              <tr key={e.id}>
                <td>{new Date(e.at).toLocaleString()}</td>
                <td>{describeChange(e)}</td>
                <td className="reason">{e.reason}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}
