import type { RuntimeStatus } from "../types";

interface StatusPillProps {
  status: RuntimeStatus;
}

const labels: Record<string, string> = {
  healthy: "Healthy",
  contract_mismatch: "Contract mismatch",
  invalid_response: "Invalid response",
  unreachable: "Unreachable",
  unknown: "Unknown",
  active: "Active",
  draft: "Draft",
  disabled: "Disabled",
  unconfigured: "Unconfigured",
};

export function StatusPill({ status }: StatusPillProps) {
  const tone = `status-pill status-pill--${status.replace(/_/g, "-")}`;
  return <span className={tone}>{labels[status] ?? status}</span>;
}
