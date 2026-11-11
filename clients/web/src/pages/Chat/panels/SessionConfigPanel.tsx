import { useEffect, useMemo, useState } from 'react';
import { useWorkflowStore } from '@/store/useAppStore';
import { useSessionStore } from '@/store/useSessionStore';
import { useDevices } from '@/pages/Chat/hooks/useDevices';
import { sessionApi, type WebRTCProviderInfo } from '@/realtime/session';
import { isSecureAudioContext } from '@/audio/secure-context';
import type { ConnectionKind } from '@/types/session';
import { Button } from '@/components/ui/button';
import { Textarea } from '@/components/ui/textarea';
import { Switch } from '@/components/ui/switch';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { AlertCircle, AlertTriangle, RefreshCw } from 'lucide-react';
import { cn } from '@/lib/utils';

interface SessionConfigPanelProps {
  busy: boolean;
}

export function SessionConfigPanel({ busy }: SessionConfigPanelProps) {
  const workflows = useWorkflowStore((s) => s.workflows);
  const fetchWorkflows = useWorkflowStore((s) => s.fetchWorkflows);
  const config = useSessionStore((s) => s.config);
  const setConfig = useSessionStore((s) => s.setConfig);
  const error = useSessionStore((s) => s.error);
  const { inputs, outputs, error: deviceError, refresh } = useDevices();

  useEffect(() => {
    if (workflows.length === 0) void fetchWorkflows();
  }, [workflows.length, fetchWorkflows]);

  const [propertiesTouched, setPropertiesTouched] = useState(false);
  const [webrtcProviders, setWebrtcProviders] = useState<WebRTCProviderInfo[]>([]);

  useEffect(() => {
    sessionApi
      .getWebRTCProviders()
      .then((res) => {
        setWebrtcProviders(res.providers || []);
        if (!config.webrtcProvider && res.default) {
          setConfig({ webrtcProvider: res.default });
        }
      })
      .catch(() => {});
  }, [config.webrtcProvider, setConfig]);

  const propertiesError = useMemo(() => {
    try {
      const parsed: unknown = JSON.parse(config.propertiesText.trim() || '{}');
      if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) {
        return '必须是一个 JSON 对象';
      }
      return undefined;
    } catch (cause) {
      return (cause as Error).message;
    }
  }, [config.propertiesText]);

  // 方案 A：扁平化可用通道选项列表
  const channelOptions = useMemo(() => {
    const list: Array<{
      key: string;
      label: string;
      description: string;
      transport: ConnectionKind;
      provider?: string;
    }> = [
      {
        key: 'websocket',
        label: 'WebSocket',
        description: '标准全双工 WebSocket 连接（二进制帧封装）',
        transport: 'websocket',
      },
    ];

    if (webrtcProviders.length <= 1) {
      list.push({
        key: 'webrtc',
        label: 'WebRTC',
        description: webrtcProviders[0]?.description || '标准 WebRTC 传输网络',
        transport: 'webrtc',
        provider: webrtcProviders[0]?.name,
      });
    } else {
      webrtcProviders.forEach((p) => {
        let shortLabel = p.label;
        if (p.name === 'wtn') shortLabel = 'WTN';
        else if (p.name === 'cloudflare') shortLabel = 'Cloudflare';
        list.push({
          key: `webrtc:${p.name}`,
          label: shortLabel,
          description: `${p.label} - ${p.description}`,
          transport: 'webrtc',
          provider: p.name,
        });
      });
    }
    return list;
  }, [webrtcProviders]);

  const currentChannelKey = useMemo(() => {
    if (config.transport === 'websocket') return 'websocket';
    if (webrtcProviders.length > 1) {
      const activeProvider = config.webrtcProvider || webrtcProviders[0]?.name;
      return `webrtc:${activeProvider}`;
    }
    return 'webrtc';
  }, [config.transport, config.webrtcProvider, webrtcProviders]);

  const activeChannel = useMemo(
    () => channelOptions.find((o) => o.key === currentChannelKey) || channelOptions[0],
    [channelOptions, currentChannelKey],
  );

  const handleChannelChange = (val: string) => {
    const opt = channelOptions.find((o) => o.key === val);
    if (!opt) return;
    setConfig({
      transport: opt.transport,
      ...(opt.provider ? { webrtcProvider: opt.provider } : {}),
    });
  };

  const locked = busy;

  return (
    <aside
      aria-label="会话配置"
      className="w-[280px] shrink-0 border-r border-border bg-card/95 backdrop-blur-md flex flex-col min-h-0"
    >
      <div className="px-3.5 py-2.5 border-b border-border/60">
        <span className="text-[10px] font-bold tracking-wider text-muted-foreground uppercase">
          SESSION CONFIG
        </span>
      </div>

      <div className="flex-1 min-h-0 overflow-y-auto p-3.5 flex flex-col gap-4 text-xs">
        {!isSecureAudioContext() && (
          <div className="flex items-start gap-2 p-2 rounded-lg bg-amber-500/10 border border-amber-500/20 text-amber-600 dark:text-amber-400 text-[11px] leading-relaxed">
            <AlertTriangle className="h-4 w-4 shrink-0 mt-0.5" />
            <div>
              非安全上下文：浏览器会隐藏音频设备并拒绝打开麦克风。请改用 <b>https</b> 或{' '}
              <b>localhost</b> 访问。
            </div>
          </div>
        )}

        {/* Workflow Selector */}
        <div>
          <label className="font-medium text-foreground mb-1 block">工作流</label>
          <Select
            value={config.workflowId}
            disabled={locked}
            onValueChange={(val) => setConfig({ workflowId: val })}
          >
            <SelectTrigger>
              <SelectValue placeholder="选择工作流..." />
            </SelectTrigger>
            <SelectContent>
              {workflows.map((wf) => (
                <SelectItem key={wf.id} value={wf.id}>
                  {wf.name || wf.id}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {workflows.length === 0 && (
            <div className="text-[10px] text-muted-foreground mt-1">暂无可用工作流</div>
          )}
        </div>

        {/* Override Properties */}
        <div>
          <label className="font-medium text-foreground mb-1 block">覆盖参数 properties</label>
          <Textarea
            rows={4}
            value={config.propertiesText}
            onChange={(e: React.ChangeEvent<HTMLTextAreaElement>) =>
              setConfig({ propertiesText: e.target.value })
            }
            onBlur={() => setPropertiesTouched(true)}
            disabled={locked}
            className={cn(
              'font-mono text-xs leading-relaxed',
              propertiesTouched && propertiesError && 'border-destructive focus-visible:ring-destructive'
            )}
            placeholder='{"deepgram_asr": {"language": "zh-CN"}}'
          />
          <div className="text-[10px] text-muted-foreground mt-1 leading-normal">
            {(propertiesTouched && propertiesError) ? (
              <span className="text-destructive font-medium">{propertiesError}</span>
            ) : (
              '注入节点参数（JSON 格式）'
            )}
          </div>
        </div>

        {/* Transport Provider */}
        <div>
          <label className="font-medium text-foreground mb-1 block">连接实现</label>
          <Tabs
            value={currentChannelKey}
            onValueChange={handleChannelChange}
            className="w-full"
          >
            <TabsList className="w-full flex h-8 p-1 bg-sunken border border-border/60 rounded-lg">
              {channelOptions.map((c) => (
                <TabsTrigger
                  key={c.key}
                  value={c.key}
                  disabled={locked}
                  title={c.description}
                  className="flex-1 text-xs py-1 h-6 font-medium data-[state=active]:bg-card data-[state=active]:text-foreground data-[state=active]:shadow-xs"
                >
                  {c.label}
                </TabsTrigger>
              ))}
            </TabsList>
          </Tabs>
          {activeChannel?.description && (
            <div className="text-[10px] text-muted-foreground mt-1.5 leading-normal">
              {activeChannel.description}
            </div>
          )}
        </div>

        <div className="h-px bg-border/60" />

        {/* Audio Devices */}
        <div>
          <div className="text-[10px] font-bold tracking-wider text-muted-foreground uppercase mb-2">
            AUDIO DEVICES
          </div>
          <div className="flex flex-col gap-2.5">
            <div>
              <label className="text-[11px] text-muted-foreground mb-1 block">输入设备（麦克风）</label>
              <Select
                value={config.inputDeviceId}
                disabled={locked}
                onValueChange={(val) => setConfig({ inputDeviceId: val })}
              >
                <SelectTrigger>
                  <SelectValue placeholder="系统默认" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="default-input">系统默认</SelectItem>
                  {inputs.map((d) => (
                    <SelectItem key={d.deviceId} value={d.deviceId}>
                      {d.label || `Microphone (${d.deviceId.slice(0, 5)})`}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div>
              <label className="text-[11px] text-muted-foreground mb-1 block">输出设备（扬声器）</label>
              <Select
                value={config.outputDeviceId}
                disabled={locked}
                onValueChange={(val) => setConfig({ outputDeviceId: val })}
              >
                <SelectTrigger>
                  <SelectValue placeholder="系统默认" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="default-output">系统默认</SelectItem>
                  {outputs.map((d) => (
                    <SelectItem key={d.deviceId} value={d.deviceId}>
                      {d.label || `Speaker (${d.deviceId.slice(0, 5)})`}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
          <div className="text-[10px] text-muted-foreground mt-1.5 leading-normal">
            浏览器的回声消除以默认输出设备为参考，换到其他设备可能失效。
          </div>
          {deviceError && (
            <div className="mt-1 flex items-center justify-between text-[11px] text-amber-500">
              <span>{deviceError}</span>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => void refresh()}
                className="h-5 px-1.5 text-[10px]"
              >
                <RefreshCw size={10} className="mr-1" /> 重试
              </Button>
            </div>
          )}
        </div>

        {/* Microphone Toggle */}
        <div className="flex items-center justify-between pt-1">
          <div>
            <div className="font-medium text-foreground">启用麦克风</div>
            <div className="text-[10px] text-muted-foreground">关闭后仅接收下行音频</div>
          </div>
          <Switch
            checked={config.captureEnabled}
            disabled={locked}
            onCheckedChange={(checked) => setConfig({ captureEnabled: checked })}
          />
        </div>
      </div>

      <div className="px-3.5 py-2.5 border-t border-border/60 bg-sunken/20">
        {error ? (
          <div className="flex items-start gap-1.5 p-2 rounded-lg bg-destructive/10 border border-destructive/20 text-destructive text-xs">
            <AlertCircle size={14} className="shrink-0 mt-0.5" />
            <span>{error}</span>
          </div>
        ) : (
          <div className="text-[10px] text-muted-foreground leading-normal">
            配置只在空闲时修改。开始对话后麦克风持续上行，轮次由服务端 VAD 判定。
          </div>
        )}
      </div>
    </aside>
  );
}
