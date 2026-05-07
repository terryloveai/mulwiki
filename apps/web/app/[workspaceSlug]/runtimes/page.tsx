"use client";

import { use, useState, useMemo, useCallback } from "react";
import { useRuntimes, useDaemons, useStopDaemon, useStartDaemon, useDaemonLogs } from "@mulwiki/core/hooks";
import type { AgentRuntime } from "@mulwiki/core/types";
import { Spinner } from "@mulwiki/ui/components/Spinner";
import { Badge } from "@mulwiki/ui/components/Badge";
import { Button } from "@mulwiki/ui/components/Button";
import { Input } from "@mulwiki/ui/components/Input";
import {
  Cpu,
  Monitor,
  Clock,
  Wifi,
  WifiOff,
  Search,
  Server,
  Activity,
  Play,
  Square,
  ScrollText,
  ChevronDown,
  ChevronRight,
  Terminal,
} from "lucide-react";

/* ── helpers ── */

function timeAgo(iso: string): string {
  const d = new Date(iso);
  const now = Date.now();
  const sec = Math.floor((now - d.getTime()) / 1000);
  if (sec < 60) return `${sec}s ago`;
  if (sec < 3600) return `${Math.floor(sec / 60)}m ago`;
  if (sec < 86400) return `${Math.floor(sec / 3600)}h ago`;
  return `${Math.floor(sec / 86400)}d ago`;
}

function uptimeSince(iso: string): string {
  const d = new Date(iso);
  const now = Date.now();
  const sec = Math.floor((now - d.getTime()) / 1000);
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

function runtimeHealth(rt: AgentRuntime, now: number): "online" | "offline" {
  if (rt.status !== "online") return "offline";
  const beat = new Date(rt.last_heartbeat).getTime();
  if (now - beat > 5 * 60 * 1000) return "offline";
  return "online";
}

function parseRuntimeIds(raw: string): number {
  try {
    const arr = JSON.parse(raw);
    return Array.isArray(arr) ? arr.length : 0;
  } catch {
    return 0;
  }
}

/* ── page ── */

export default function RuntimesPage({
  params,
}: {
  params: Promise<{ workspaceSlug: string }>;
}) {
  const { workspaceSlug } = use(params);
  const { data: runtimes = [], isLoading: runtimesLoading, error: runtimesError } = useRuntimes(workspaceSlug);
  const { data: daemons = [], isLoading: daemonsLoading } = useDaemons();
  const [search, setSearch] = useState("");
  const [expandedDaemons, setExpandedDaemons] = useState<Set<string>>(new Set());
  const [viewingLogs, setViewingLogs] = useState<string | null>(null);

  const stopDaemon = useStopDaemon();
  const startDaemon = useStartDaemon(workspaceSlug);
  const { data: logsData } = useDaemonLogs(viewingLogs);

  const now = Date.now();

  // Build daemon-runtime map
  const daemonMap = useMemo(() => {
    const m = new Map<string, { runtimes: AgentRuntime[]; online: number }>();
    // Pre-populate from daemon list
    for (const d of daemons) {
      const rts = runtimes.filter((r) => r.daemon_id === d.id);
      m.set(d.id, {
        runtimes: rts,
        online: rts.filter((r) => runtimeHealth(r, now) === "online").length,
      });
    }
    // Add orphaned runtimes
    const orphaned = runtimes.filter(
      (r) => !r.daemon_id || !daemons.some((d) => d.id === r.daemon_id)
    );
    if (orphaned.length > 0) {
      m.set("__orphaned__", {
        runtimes: orphaned,
        online: orphaned.filter((r) => runtimeHealth(r, now) === "online").length,
      });
    }
    return m;
  }, [daemons, runtimes, now]);

  const toggleDaemon = useCallback((id: string) => {
    setExpandedDaemons((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  const onlineCount = runtimes.filter((r) => runtimeHealth(r, now) === "online").length;
  const offlineCount = runtimes.length - onlineCount;

  const isLoading = runtimesLoading || daemonsLoading;

  if (isLoading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Spinner />
      </div>
    );
  }

  if (runtimesError) {
    return (
      <div className="rounded-lg border border-red-200 bg-red-50 p-6 text-red-700">
        Failed to load runtimes. Make sure the server is running.
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-5xl">
      {/* Page Header */}
      <div className="mb-6 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Server className="h-4 w-4 text-muted-foreground" />
          <h1 className="text-lg font-semibold">Runtimes</h1>
          <span className="font-mono text-xs tabular-nums text-muted-foreground/70">
            {runtimes.length}
          </span>
        </div>
      </div>

      {/* Quick stats */}
      {runtimes.length > 0 && (
        <div className="mb-4 flex items-center gap-3 text-xs text-muted-foreground">
          <span className="flex items-center gap-1">
            <span className="h-2 w-2 rounded-full bg-emerald-500" />
            {onlineCount} online
          </span>
          <span className="flex items-center gap-1">
            <span className="h-2 w-2 rounded-full bg-muted-foreground/40" />
            {offlineCount} offline
          </span>
          <span>{daemons.length} daemon{daemons.length !== 1 ? "s" : ""}</span>
        </div>
      )}

      {/* ── Daemon Grouped Cards ── */}
      {(() => {
        const hasLiveDaemon = Array.from(daemonMap.keys()).some((key) => {
          if (key === "__orphaned__") return false;
          const d = daemons.find((dd) => dd.id === key);
          return d && new Date(d.last_heartbeat).getTime() > now - 5 * 60 * 1000 && d.pid > 0;
        });

        if (!hasLiveDaemon && runtimes.length === 0) {
          return (
            <div className="flex h-64 flex-col items-center justify-center gap-3 text-muted-foreground">
              <Cpu className="h-12 w-12 opacity-30" />
              <p className="text-lg font-medium">No runtimes registered</p>
              <p className="text-sm">
                Run <code className="rounded bg-muted px-1 py-0.5 text-xs">mulwiki daemon start --workspace {workspaceSlug}</code> to auto-register.
              </p>
            </div>
          );
        }

        if (!hasLiveDaemon) {
          return (
            <div className="mb-4 flex flex-col items-center gap-3 rounded-lg border bg-card p-8 text-muted-foreground">
              <Server className="h-8 w-8 opacity-30" />
              <p className="text-sm font-medium">No daemon running</p>
              <p className="text-xs">
                Start a daemon to auto-detect and register runtimes.
              </p>
              <Button
                size="sm"
                onClick={() => startDaemon.mutate({})}
                disabled={startDaemon.isPending}
              >
                <Play className="mr-1 h-3.5 w-3.5" />
                {startDaemon.isPending ? "Starting..." : "Start Daemon"}
              </Button>
            </div>
          );
        }

        return null;
      })()}

      {/* Render daemon cards when live daemons or orphaned runtimes exist */}
      {(Array.from(daemonMap.keys()).some((key) => {
        if (key === "__orphaned__") return daemonMap.get("__orphaned__")!.runtimes.length > 0;
        const d = daemons.find((dd) => dd.id === key);
        return d && new Date(d.last_heartbeat).getTime() > now - 5 * 60 * 1000 && d.pid > 0;
      })) && (
        <div className="space-y-3">
          {Array.from(daemonMap.entries())
            .filter(([daemonID]) => {
              if (daemonID === "__orphaned__") return true;
              const d = daemons.find((dd) => dd.id === daemonID);
              return d && new Date(d.last_heartbeat).getTime() > now - 5 * 60 * 1000 && d.pid > 0;
            })
            .map(([daemonID, { runtimes: rts, online }]) => {
            const daemon = daemons.find((d) => d.id === daemonID);
            const isAlive = daemon
              ? new Date(daemon.last_heartbeat).getTime() > now - 5 * 60 * 1000
              : false;
            const isExpanded = expandedDaemons.has(daemonID);
            const isOrphan = daemonID === "__orphaned__";

            return (
              <div
                key={daemonID}
                className="overflow-hidden rounded-lg border bg-card"
              >
                {/* Daemon Header */}
                <div className="flex items-center justify-between px-4 py-3">
                  <div className="flex items-center gap-3">
                    {!isOrphan && (
                      <button
                        onClick={() => toggleDaemon(daemonID)}
                        className="text-muted-foreground hover:text-foreground"
                      >
                        {isExpanded ? (
                          <ChevronDown className="h-4 w-4" />
                        ) : (
                          <ChevronRight className="h-4 w-4" />
                        )}
                      </button>
                    )}
                    <Server className={`h-4 w-4 ${isOrphan ? "text-muted-foreground/40" : "text-muted-foreground"}`} />
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-medium">
                          {isOrphan ? "Orphaned Runtimes" : daemon?.hostname || daemonID.slice(0, 8)}
                        </span>
                        {!isOrphan && (
                          <Badge
                            variant={isAlive ? "success" : "secondary"}
                            className="text-[10px] px-1.5 py-0"
                          >
                            {isAlive ? "Running" : "Offline"}
                          </Badge>
                        )}
                      </div>
                      <div className="mt-0.5 text-xs text-muted-foreground">
                        {isOrphan
                          ? `${rts.length} runtime${rts.length !== 1 ? "s" : ""}, no daemon`
                          : `${rts.length} runtime${rts.length !== 1 ? "s" : ""} · ${online} online`
                        }
                        {daemon?.last_heartbeat && isAlive && (
                          <span className="ml-2">· up {uptimeSince(daemon.last_heartbeat)}</span>
                        )}
                        {daemon && !isAlive && daemon.last_heartbeat && (
                          <span className="ml-2">· last seen {timeAgo(daemon.last_heartbeat)}</span>
                        )}
                      </div>
                    </div>
                  </div>

                  {/* Controls */}
                  {!isOrphan && (
                    <div className="flex items-center gap-1">
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => setViewingLogs(viewingLogs === daemonID ? null : daemonID)}
                      >
                        <ScrollText className="mr-1 h-3.5 w-3.5" />
                        Logs
                      </Button>
                      {isAlive ? (
                        <Button
                          size="sm"
                          variant="ghost"
                          className="text-red-600 hover:text-red-700"
                          onClick={() => stopDaemon.mutate(daemonID)}
                          disabled={stopDaemon.isPending}
                        >
                          <Square className="mr-1 h-3.5 w-3.5" />
                          Stop
                        </Button>
                      ) : (
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => startDaemon.mutate({})}
                          disabled={startDaemon.isPending}
                        >
                          <Play className="mr-1 h-3.5 w-3.5" />
                          Start
                        </Button>
                      )}
                    </div>
                  )}
                </div>

                {/* Log viewer */}
                {viewingLogs === daemonID && logsData && (
                  <div className="border-t bg-muted/30 px-4 py-2">
                    <div className="mb-1 flex items-center justify-between">
                      <span className="flex items-center gap-1 text-xs text-muted-foreground">
                        <Terminal className="h-3 w-3" />
                        {logsData.log_path}
                      </span>
                      <span className="text-xs text-muted-foreground">{logsData.total} lines</span>
                    </div>
                    <pre className="max-h-48 overflow-auto rounded border bg-black/90 p-3 font-mono text-[11px] leading-relaxed text-green-400">
                      {logsData.lines.length > 0
                        ? logsData.lines.join("\n")
                        : "(empty)"}
                    </pre>
                  </div>
                )}

                {/* Runtime Table */}
                {isExpanded && rts.length > 0 && (
                  <div className="border-t">
                    <table className="w-full">
                      <thead>
                        <tr className="border-b bg-muted/50 text-[11px] text-muted-foreground">
                          <th className="px-4 py-2 text-left font-medium">Runtime</th>
                          <th className="px-4 py-2 text-left font-medium">Backend</th>
                          <th className="px-4 py-2 text-left font-medium">Machine</th>
                          <th className="px-4 py-2 text-left font-medium">Version</th>
                          <th className="px-4 py-2 text-left font-medium">Last Beat</th>
                          <th className="px-4 py-2 text-center font-medium">Status</th>
                        </tr>
                      </thead>
                      <tbody>
                        {rts.map((rt) => {
                          const health = runtimeHealth(rt, now);
                          return (
                            <tr
                              key={rt.id}
                              className="border-b text-sm last:border-b-0 hover:bg-muted/30"
                            >
                              <td className="px-4 py-2 font-medium">
                                <div className="flex items-center gap-2">
                                  <Cpu className="h-3.5 w-3.5 text-muted-foreground" />
                                  {rt.name}
                                </div>
                              </td>
                              <td className="px-4 py-2 font-mono text-xs">{rt.backend}</td>
                              <td className="px-4 py-2 text-xs text-muted-foreground">
                                <span className="inline-flex items-center gap-1">
                                  <Monitor className="h-3 w-3" />
                                  {rt.hostname || "—"}
                                </span>
                              </td>
                              <td className="px-4 py-2 font-mono text-xs text-muted-foreground">
                                {rt.version || "—"}
                              </td>
                              <td className="px-4 py-2 text-xs text-muted-foreground">
                                {rt.last_heartbeat ? timeAgo(rt.last_heartbeat) : "never"}
                              </td>
                              <td className="px-4 py-2 text-center">
                                <Badge
                                  variant={health === "online" ? "success" : "secondary"}
                                  className="text-[10px] px-1.5 py-0"
                                >
                                  {health === "online" ? (
                                    <Wifi className="mr-1 h-3 w-3" />
                                  ) : (
                                    <WifiOff className="mr-1 h-3 w-3" />
                                  )}
                                  {health}
                                </Badge>
                              </td>
                            </tr>
                          );
                        })}
                      </tbody>
                    </table>
                  </div>
                )}

                {/* Expand hint when collapsed */}
                {!isExpanded && !isOrphan && rts.length > 0 && (
                  <div className="border-t px-4 py-1.5">
                    <span className="text-xs text-muted-foreground">
                      {rts.map((r) => r.name).join(", ")}
                    </span>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
