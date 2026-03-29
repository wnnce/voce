package plugins

import (
	_ "github.com/wnnce/voce/internal/plugins/benchmark"
	_ "github.com/wnnce/voce/internal/plugins/caption"
	_ "github.com/wnnce/voce/internal/plugins/deepgram_asr"
	_ "github.com/wnnce/voce/internal/plugins/elevenlabs_tts"
	_ "github.com/wnnce/voce/internal/plugins/google_asr"
	_ "github.com/wnnce/voce/internal/plugins/interrupter"
	_ "github.com/wnnce/voce/internal/plugins/md_filter"
	_ "github.com/wnnce/voce/internal/plugins/minimax_tts"
	_ "github.com/wnnce/voce/internal/plugins/openai_llm"
	_ "github.com/wnnce/voce/internal/plugins/openai_tts"
	_ "github.com/wnnce/voce/internal/plugins/qwen_asr"
	_ "github.com/wnnce/voce/internal/plugins/sink"
)
