import React, { useCallback, useEffect, useMemo, useState } from 'react';
import CodeMirror from '@uiw/react-codemirror';
import { json } from '@codemirror/lang-json';
import {
  Mic,
  PhoneOff,
  X,
  AlertCircle,
  Settings,
  Sparkles,
  Workflow as WorkflowIcon,
  Braces,
} from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Tooltip } from '@/components/ui/tooltip';
import { Switch } from '@/components/ui/switch';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { SignalBadge, type SignalTone } from '@/pages/Chat/components/SignalBadge';
import { LevelMeter } from '@/pages/Chat/components/LevelMeter';
import { ConversationStream } from '@/pages/Chat/panels/ConversationStream';
import { useSessionController } from '@/pages/Chat/hooks/useSessionController';
import { useDevices } from '@/pages/Chat/hooks/useDevices';
import { useThemeControl } from '@/hooks/useThemeControl';
import { useSessionStore } from '@/store/useSessionStore';
import type { ConnectionState } from '@/types/session';
import type { WorkflowConfig } from '@/types/workflow';
import { cn } from '@/lib/utils';

interface StudioPlaygroundProps {
  open: boolean;
  onClose: () => void;
  workflow: WorkflowConfig | null;
  width?: number;
}

const stateSignal: Record<ConnectionState, { tone: SignalTone; label: string }> = {
  idle: { tone: 'neutral', label: 'Idle' },
  creating: { tone: 'warn', label: 'Creating' },
  connecting: { tone: 'warn', label: 'Connecting' },
  active: { tone: 'ok', label: 'Active' },
  reconnecting: { tone: 'warn', label: 'Reconnecting' },
  closing: { tone: 'neutral', label: 'Hanging up' },
  closed: { tone: 'neutral', label: 'Disconnected' },
  error: { tone: 'danger', label: 'Error' },
};

export const StudioPlayground: React.FC<StudioPlaygroundProps> = ({
  open,
  onClose,
  workflow,
}) => {
  const { start, stop, busy } = useSessionController();
  const { inputs, outputs } = useDevices();
  const { mode } = useThemeControl();

  const state = useSessionStore((s) => s.state);
  const stateDetail = useSessionStore((s) => s.stateDetail);
  const sessionId = useSessionStore((s) => s.sessionId);
  const config = useSessionStore((s) => s.config);
  const setConfig = useSessionStore((s) => s.setConfig);
  const uplinkRms = useSessionStore((s) => s.uplinkRms);

  const [showSettings, setShowSettings] = useState(false);
  const [showOverrides, setShowOverrides] = useState(false);

  // Automatically keep session workflowId synchronized with active workflow
  useEffect(() => {
    if (!workflow?.id) return;
    if (config.workflowId !== workflow.id) {
      if (busy) {
        void stop();
      }
      useSessionStore.getState().reset();
      useSessionStore.getState().setConfig({ workflowId: workflow.id });
    }
  }, [workflow?.id, config.workflowId, busy, stop]);

  // Validate properties JSON text
  const propertiesError = useMemo(() => {
    const raw = (config.propertiesText || '').trim();
    if (!raw || raw === '{}') return null;
    try {
      const parsed: unknown = JSON.parse(raw);
      if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) {
        return '必须为 JSON 对象（例如 {"node_name": {...}}）';
      }
      return null;
    } catch (err) {
      return (err as Error).message;
    }
  }, [config.propertiesText]);

  // Whether custom overrides are actively populated
  const hasActiveOverrides = useMemo(() => {
    const raw = (config.propertiesText || '').trim();
    return Boolean(raw && raw !== '{}' && !propertiesError);
  }, [config.propertiesText, propertiesError]);

  // Insert template based on current workflow nodes
  const handleInsertTemplate = useCallback(() => {
    if (!workflow?.nodes || workflow.nodes.length === 0) {
      setConfig({ propertiesText: '{\n  "node_name": {\n    "key": "value"\n  }\n}' });
      return;
    }
    const templateObj: Record<string, Record<string, unknown>> = {};
    for (const node of workflow.nodes) {
      const name = node.name || node.id;
      templateObj[name] = {};
    }
    setConfig({ propertiesText: JSON.stringify(templateObj, null, 2) });
  }, [workflow, setConfig]);

  // Format current JSON
  const handleFormatJson = useCallback(() => {
    try {
      const parsed = JSON.parse(config.propertiesText || '{}');
      setConfig({ propertiesText: JSON.stringify(parsed, null, 2) });
    } catch {
      // ignore if invalid
    }
  }, [config.propertiesText, setConfig]);

  // Reset overrides to empty
  const handleResetOverrides = useCallback(() => {
    setConfig({ propertiesText: '{}' });
  }, [setConfig]);

  const userSpeaking = useSessionStore((s) =>
    s.spans.some((span) => span.role === 'user' && span.end === undefined)
  );
  const agentSpeaking = useSessionStore((s) =>
    s.spans.some((span) => span.role === 'agent' && span.end === undefined)
  );

  const signal = stateSignal[state];
  const canStart = !busy && Boolean(workflow?.id) && !propertiesError;

  if (!open) return null;

  return (
    <aside className="flex flex-col h-full bg-card select-none overflow-hidden text-xs">
      {/* Playground Header Bar */}
      <div className="h-[42px] px-3.5 border-b border-border/80 flex items-center justify-between bg-card shrink-0">
        <div className="flex items-center gap-2 min-w-0">
          <div className="w-6 h-6 rounded-md bg-emerald-500/10 text-emerald-500 flex items-center justify-center shrink-0">
            <Sparkles size={14} />
          </div>
          <span className="text-xs font-bold text-foreground truncate">Voice Playground</span>
          {workflow?.name && (
            <div
              className="hidden sm:flex items-center gap-1 px-1.5 py-0.5 rounded bg-sunken border border-border/70 text-[10px] font-medium text-foreground max-w-[120px] truncate"
              title={`Active Workflow: ${workflow.name}`}
            >
              <WorkflowIcon size={10} className="text-primary shrink-0" />
              <span className="truncate">{workflow.name}</span>
            </div>
          )}
          <SignalBadge tone={signal.tone} label={signal.label} detail={stateDetail} />
        </div>

        <div className="flex items-center gap-1">
          {/* Overrides JSON Toggle */}
          <Tooltip title={showOverrides ? '收起参数覆盖' : '覆盖节点参数 (Properties JSON)'}>
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={() => {
                setShowOverrides((prev) => !prev);
                if (!showOverrides) setShowSettings(false);
              }}
              className={cn(
                'h-6 w-6 relative text-muted-foreground hover:text-foreground',
                showOverrides && 'bg-sunken text-primary',
                hasActiveOverrides && !showOverrides && 'text-primary'
              )}
              aria-label="Toggle Property Overrides"
            >
              <Braces size={13} />
              {hasActiveOverrides && (
                <span className="absolute top-1 right-1 w-1.5 h-1.5 rounded-full bg-primary" />
              )}
            </Button>
          </Tooltip>

          {/* Audio Settings Toggle */}
          <Tooltip title={showSettings ? '隐藏音频设备设置' : '音频输入/输出设备设置'}>
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={() => {
                setShowSettings((prev) => !prev);
                if (!showSettings) setShowOverrides(false);
              }}
              className={cn(
                'h-6 w-6 text-muted-foreground hover:text-foreground',
                showSettings && 'bg-sunken text-foreground'
              )}
              aria-label="Toggle Audio Devices"
            >
              <Settings size={13} />
            </Button>
          </Tooltip>

          {/* Close */}
          <Tooltip title="关闭测试面板">
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={onClose}
              className="h-6 w-6 text-muted-foreground hover:text-foreground"
              aria-label="Close Playground"
            >
              <X size={14} />
            </Button>
          </Tooltip>
        </div>
      </div>

      {/* Top Controls: Start/Hangup + Live RMS Meter */}
      <div className="p-3 border-b border-border/70 flex flex-col gap-2 bg-card/60 shrink-0">
        {/* Active Workflow Identity Indicator */}
        <div className="flex items-center justify-between text-[11px] px-0.5 text-muted-foreground select-none">
          <div className="flex items-center gap-1.5 min-w-0">
            <span className="text-[10px] uppercase font-mono tracking-wider text-muted-foreground/80">Target:</span>
            <span className="font-semibold text-foreground truncate max-w-[140px]">
              {workflow?.name || 'No Workflow Selected'}
            </span>
            {workflow?.version && (
              <span className="text-[10px] font-mono text-muted-foreground/60 shrink-0">v{workflow.version}</span>
            )}
            {hasActiveOverrides && (
              <button
                type="button"
                onClick={() => {
                  setShowOverrides(true);
                  setShowSettings(false);
                }}
                className="px-1.5 py-0.2 rounded bg-primary/10 text-primary border border-primary/25 text-[9px] font-mono font-medium hover:bg-primary/20 transition-colors cursor-pointer"
                title="已设置参数覆盖，点击展开编辑"
              >
                PROPS
              </button>
            )}
          </div>
          <span className="text-[10px] font-mono text-muted-foreground/70 shrink-0">
            {workflow?.nodes?.length ?? 0} nodes
          </span>
        </div>

        {!workflow?.id ? (
          <div className="p-2 rounded-lg bg-amber-500/10 border border-amber-500/20 text-amber-600 dark:text-amber-400 text-[11px] flex items-start gap-1.5">
            <AlertCircle size={13} className="shrink-0 mt-0.5" />
            <span>请先在左侧选择或保存工作流。</span>
          </div>
        ) : null}

        <div className="flex items-center gap-2">
          {busy ? (
            <Button
              variant="destructive"
              size="sm"
              onClick={() => void stop()}
              className="flex-1 h-8 text-xs font-semibold gap-1.5 shadow-xs"
            >
              <PhoneOff size={13} />
              <span>Hang up</span>
            </Button>
          ) : (
            <Button
              variant="default"
              size="sm"
              onClick={() => void start()}
              disabled={!canStart}
              className="flex-1 h-8 text-xs font-semibold gap-1.5 bg-emerald-600 hover:bg-emerald-700 text-white shadow-xs"
            >
              <Mic size={13} />
              <span>Start Voice Call</span>
            </Button>
          )}

          {sessionId && (
            <span className="text-[10px] font-mono text-muted-foreground/80 px-2 py-1 rounded bg-sunken border border-border shrink-0">
              sid:{sessionId.slice(0, 6)}
            </span>
          )}
        </div>

        {/* Live RMS Meter */}
        <div className="flex flex-col gap-1 pt-1">
          <div className="flex items-center justify-between text-[10px] text-muted-foreground font-mono">
            <span>MIC INPUT</span>
            <span>{config.captureEnabled ? (userSpeaking ? 'SPEAKING' : 'IDLE') : 'MUTED'}</span>
          </div>
          <LevelMeter value={uplinkRms} active={uplinkRms > 0.05} />
        </div>

        {/* Property Overrides JSON Editor Panel */}
        {showOverrides && (
          <div className="mt-1 p-2.5 rounded-lg border border-border/80 bg-sunken/60 flex flex-col gap-2 animate-in fade-in-50">
            <div className="flex items-center justify-between text-[11px]">
              <div className="flex items-center gap-1.5">
                <Braces size={12} className="text-primary" />
                <span className="font-semibold text-foreground uppercase tracking-wider text-[10px]">
                  覆盖参数 properties (JSON)
                </span>
              </div>
              <div className="flex items-center gap-1">
                <button
                  type="button"
                  onClick={handleInsertTemplate}
                  disabled={busy}
                  className="text-[10px] text-muted-foreground hover:text-foreground px-1.5 py-0.5 rounded bg-card hover:bg-card-hover border border-border/60 transition-colors cursor-pointer disabled:opacity-40"
                  title="根据当前工作流各节点生成模板"
                >
                  节点模板
                </button>
                <button
                  type="button"
                  onClick={handleFormatJson}
                  disabled={busy || Boolean(propertiesError)}
                  className="text-[10px] text-muted-foreground hover:text-foreground px-1.5 py-0.5 rounded bg-card hover:bg-card-hover border border-border/60 transition-colors cursor-pointer disabled:opacity-40"
                  title="格式化 JSON"
                >
                  格式化
                </button>
                <button
                  type="button"
                  onClick={handleResetOverrides}
                  disabled={busy}
                  className="text-[10px] text-muted-foreground hover:text-destructive px-1.5 py-0.5 rounded bg-card hover:bg-card-hover border border-border/60 transition-colors cursor-pointer disabled:opacity-40"
                  title="重置为空对象 {}"
                >
                  清空
                </button>
              </div>
            </div>

            <div
              className={cn(
                'rounded-md border overflow-hidden text-xs bg-background',
                propertiesError ? 'border-destructive ring-1 ring-destructive/40' : 'border-border/80'
              )}
            >
              <CodeMirror
                value={config.propertiesText || '{}'}
                height="150px"
                theme={mode === 'dark' ? 'dark' : 'light'}
                extensions={[json()]}
                onChange={(val) => setConfig({ propertiesText: val })}
                editable={!busy}
                className="font-mono text-xs"
              />
            </div>

            {propertiesError ? (
              <div className="flex items-center gap-1.5 text-[10px] text-destructive bg-destructive/10 border border-destructive/20 p-1.5 rounded">
                <AlertCircle size={12} className="shrink-0" />
                <span className="truncate">{propertiesError}</span>
              </div>
            ) : (
              <div className="text-[10px] text-muted-foreground leading-tight flex items-center justify-between">
                <span>Key 为节点名称，发起通话时 deep-merge 到节点配置</span>
                {hasActiveOverrides && <span className="text-emerald-500 font-medium shrink-0">已就绪</span>}
              </div>
            )}
          </div>
        )}

        {/* Audio Device Settings Dropdown Form */}
        {showSettings && (
          <div className="mt-1 p-2.5 rounded-lg border border-border/80 bg-sunken/60 flex flex-col gap-2 animate-in fade-in-50">
            <div className="flex flex-col gap-1">
              <label className="text-[10px] font-semibold text-muted-foreground uppercase">
                Input Microphone
              </label>
              <Select
                value={config.inputDeviceId}
                disabled={busy}
                onValueChange={(val) => setConfig({ inputDeviceId: val })}
              >
                <SelectTrigger className="h-6 text-[11px]">
                  <SelectValue placeholder="System Default" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="default-input">System Default</SelectItem>
                  {inputs.map((d) => (
                    <SelectItem key={d.deviceId} value={d.deviceId}>
                      {d.label || `Mic (${d.deviceId.slice(0, 5)})`}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="flex flex-col gap-1">
              <label className="text-[10px] font-semibold text-muted-foreground uppercase">
                Output Speaker
              </label>
              <Select
                value={config.outputDeviceId}
                disabled={busy}
                onValueChange={(val) => setConfig({ outputDeviceId: val })}
              >
                <SelectTrigger className="h-6 text-[11px]">
                  <SelectValue placeholder="System Default" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="default-output">System Default</SelectItem>
                  {outputs.map((d) => (
                    <SelectItem key={d.deviceId} value={d.deviceId}>
                      {d.label || `Speaker (${d.deviceId.slice(0, 5)})`}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="flex items-center justify-between pt-1">
              <span className="text-[11px]">Enable Microphone</span>
              <Switch
                checked={config.captureEnabled}
                disabled={busy}
                onCheckedChange={(checked) => setConfig({ captureEnabled: checked })}
              />
            </div>
          </div>
        )}
      </div>

      {/* Full-Height Vertical Conversation Stream */}
      <div className="flex-1 flex flex-col min-w-0 min-h-0 bg-chat relative overflow-hidden">
        <ConversationStream agentSpeaking={agentSpeaking} userSpeaking={userSpeaking} />
      </div>
    </aside>
  );
};
