# 内置插件列表 (Supported Plugins)

Voce 采用插件化服务，通过不同的插件组合实现多模态能力的链路编排。目前内置支持以下插件：

---

### 1. 语音识别 (ASR)

用于将实时音频流转换为文本。

- **qwen_asr**: 阿里通义千问实时语音识别。
- **deepgram_asr**: Deepgram 高效率实时语音识别，支持多种语言。
- **google_asr**: Google Cloud Speech-to-Text。

### 2. 文本大模型 (LLM)

负责处理对话逻辑与文本生成。

- **openai_llm**: 支持 OpenAI 协议的 LLM（包括 GPT-4o, DeepSeek, Kimi 等兼容协议）。

### 3. 语音合成 (TTS)

将文本转换为实时音频流。

- **minimax_tts**: MiniMax 实时语音合成。
- **elevenlabs_tts**: ElevenLabs 语音合成，支持 `previous_text` 上下文衔接。
- **openai_tts**: OpenAI 原生语音合成。

### 4. 流程控制与辅助 (Control & Utils)

- **interrupter**: 实时打断控制器，负责发送打断信号。
- **caption**: 字幕传输插件，负责实时下发 ASR 或 LLM 产生的文本内容。
- **markdown_filter**: 实时过滤文本中的 Markdown 标记代码（如标题、粗体、链接、代码块等），通常用于 TTS 前置转换。
- **sink**: 统一数据出口。

---

### 如何开发插件？

如果需要接入新的 ASR/TTS 服务商，可以参考 [插件开发指南 (Plugin Development)](plugin.md)。
