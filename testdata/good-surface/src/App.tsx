import { useSurfaceQuery, useInstance } from "@dakasa-yggdrasil/surface-toolkit";

export function App() {
  const instance = useInstance();
  const { data } = useSurfaceQuery<{ rows: unknown[] }>({
    instanceId: instance.id,
    viewId: "recommendations",
  });

  async function approveVacationRequest(id: string) {
    // Pure dispatch — decision lives in the adapter.
    await fetch(`/api/v1/integrations/${instance.id}/surface-action`, {
      method: "POST",
      credentials: "include",
      body: JSON.stringify({ action: "approve", id }),
    });
  }

  return (
    <div>
      <button onClick={() => approveVacationRequest("draft")}>Approve</button>
      <pre>{JSON.stringify(data?.rows ?? [], null, 2)}</pre>
    </div>
  );
}
