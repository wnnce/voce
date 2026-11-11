package translator

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/schema"
)

type SpeakerContext struct {
	speaker              string
	mu                   sync.Mutex
	activeTurnID         uint64
	currentDraft         string
	lastDraftTranslation string // Cached draft translation to serve as consistency reference for LLM
	draftVersion         uint64
	latestReceivedVer    uint64
	debounceTimer        *time.Timer
	cancelInflight       context.CancelFunc
}

func NewSpeakerContext(speaker string) *SpeakerContext {
	return &SpeakerContext{
		speaker: speaker,
	}
}

func (sc *SpeakerContext) Reset() {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if sc.debounceTimer != nil {
		sc.debounceTimer.Stop()
		sc.debounceTimer = nil
	}
	if sc.cancelInflight != nil {
		sc.cancelInflight()
		sc.cancelInflight = nil
	}
	sc.currentDraft = ""
	sc.lastDraftTranslation = ""
	sc.draftVersion = 0
	sc.latestReceivedVer = 0
}

func (sc *SpeakerContext) resolveLanguages(plugin *Plugin, trans *schema.Transcription) (string, string) {
	sourceLang := plugin.cfg.SourceLang
	if trans.LanguageCode != "" && (sourceLang == "" || strings.EqualFold(sourceLang, "auto")) {
		sourceLang = trans.LanguageCode
	}
	targetLang := plugin.cfg.TargetLang
	if trans.Metadata != nil {
		if t, ok := trans.Metadata["target_lang"].(string); ok && t != "" {
			targetLang = t
		}
	}
	return sourceLang, targetLang
}

func (sc *SpeakerContext) checkTurnChange(turnID uint64) {
	if turnID > 0 && turnID != sc.activeTurnID {
		if sc.debounceTimer != nil {
			sc.debounceTimer.Stop()
			sc.debounceTimer = nil
		}
		if sc.cancelInflight != nil {
			sc.cancelInflight()
			sc.cancelInflight = nil
		}
		sc.currentDraft = ""
		sc.lastDraftTranslation = ""
		sc.draftVersion = 0
		sc.latestReceivedVer = 0
		sc.activeTurnID = turnID
	}
}

func (sc *SpeakerContext) emitCaption(
	flow engine.Flow,
	tid uint64,
	metadata map[string]any,
	text string,
	translation string,
	isFinal bool,
) {
	caption := &schema.Caption{
		BaseSchema: schema.BaseSchema{
			TurnID:   tid,
			Metadata: metadata,
		},
		Text:        text,
		Translation: translation,
		Role:        schema.RoleUser,
		Speaker:     sc.speaker,
		IsFinal:     isFinal,
	}

	payload := schema.NewPayload(schema.PayloadCaption)
	if err := payload.Pack(caption); err == nil {
		_ = payload.Set("caption", caption)
		flow.SendPayload(payload.ReadOnly())
	}
}

func (sc *SpeakerContext) HandleASR(
	ctx context.Context,
	flow engine.Flow,
	trans *schema.Transcription,
	plugin *Plugin,
) {
	sc.mu.Lock()

	sourceLang, targetLang := sc.resolveLanguages(plugin, trans)
	sc.checkTurnChange(trans.TurnID)

	if trans.Final {
		sc.handleFinalASR(flow, trans, plugin, sourceLang, targetLang)
		return
	}

	sc.handleInterimASR(flow, trans, plugin, sourceLang, targetLang)
}

func (sc *SpeakerContext) handleFinalASR(
	flow engine.Flow,
	trans *schema.Transcription,
	plugin *Plugin,
	sourceLang, targetLang string,
) {
	if sc.debounceTimer != nil {
		sc.debounceTimer.Stop()
		sc.debounceTimer = nil
	}
	if sc.cancelInflight != nil {
		sc.cancelInflight()
		sc.cancelInflight = nil
	}

	finalText := trans.Text
	if finalText == "" {
		finalText = sc.currentDraft
	}
	turnID := sc.activeTurnID
	draftRef := sc.lastDraftTranslation

	sc.currentDraft = ""
	sc.lastDraftTranslation = ""
	sc.draftVersion = 0
	sc.latestReceivedVer = 0
	sc.mu.Unlock()

	if strings.TrimSpace(finalText) == "" {
		return
	}

	plugin.wg.Add(1)
	go func(text string, tid uint64, ref string) {
		defer plugin.wg.Done()
		provider := plugin.finalProvider
		if provider == nil {
			provider = plugin.fastProvider
		}

		reqCtx, cancel := context.WithTimeout(plugin.ctx, 15*time.Second)
		defer cancel()

		opts := TranslateOptions{
			SourceLang: sourceLang,
			TargetLang: targetLang,
			IsFinal:    true,
			DraftRef:   ref,
		}

		_ = provider.TranslateStream(reqCtx, text, opts,
			func(delta string, accumulated string, isDone bool) error {
				sc.emitCaption(flow, tid, trans.Metadata, text, accumulated, isDone)
				return nil
			})
	}(finalText, turnID, draftRef)
}

func (sc *SpeakerContext) handleInterimASR(
	flow engine.Flow,
	trans *schema.Transcription,
	plugin *Plugin,
	sourceLang, targetLang string,
) {
	sc.currentDraft = trans.Text
	if strings.TrimSpace(sc.currentDraft) == "" {
		sc.mu.Unlock()
		return
	}

	if plugin.cfg.SplitSentences {
		if prefix, remainder, ok := splitAtStrongPunctuation(sc.currentDraft); ok {
			sc.currentDraft = remainder
			turnID := sc.activeTurnID
			draftRef := sc.lastDraftTranslation
			sc.lastDraftTranslation = ""
			sc.mu.Unlock()

			plugin.wg.Add(1)
			go func(text string, tid uint64, ref string) {
				defer plugin.wg.Done()
				provider := plugin.finalProvider
				if provider == nil {
					provider = plugin.fastProvider
				}
				reqCtx, cancel := context.WithTimeout(plugin.ctx, 10*time.Second)
				defer cancel()

				opts := TranslateOptions{
					SourceLang: sourceLang,
					TargetLang: targetLang,
					IsFinal:    true,
					DraftRef:   ref,
				}

				_ = provider.TranslateStream(reqCtx, text, opts,
					func(delta string, accumulated string, isDone bool) error {
						sc.emitCaption(flow, tid, nil, text, accumulated, isDone)
						return nil
					})
			}(prefix, turnID, draftRef)
			return
		}
	}

	sc.scheduleDebouncedDraft(flow, plugin, sourceLang, targetLang)
	sc.mu.Unlock()
}

func (sc *SpeakerContext) scheduleDebouncedDraft(
	flow engine.Flow,
	plugin *Plugin,
	sourceLang, targetLang string,
) {
	if sc.debounceTimer != nil {
		sc.debounceTimer.Stop()
	}

	debounceMs := plugin.cfg.DebounceMs
	if debounceMs <= 0 {
		debounceMs = 250
	}

	sc.debounceTimer = time.AfterFunc(time.Duration(debounceMs)*time.Millisecond, func() {
		sc.mu.Lock()
		textToTranslate := sc.currentDraft
		if strings.TrimSpace(textToTranslate) == "" {
			sc.mu.Unlock()
			return
		}

		sc.draftVersion++
		version := sc.draftVersion
		turnID := sc.activeTurnID

		if sc.cancelInflight != nil {
			sc.cancelInflight()
		}
		draftCtx, cancel := context.WithTimeout(plugin.ctx, 5*time.Second)
		sc.cancelInflight = cancel
		sc.mu.Unlock()

		plugin.wg.Add(1)
		go func(ver uint64, text string, tid uint64) {
			defer plugin.wg.Done()
			opts := TranslateOptions{
				SourceLang: sourceLang,
				TargetLang: targetLang,
				IsFinal:    false,
			}

			_ = plugin.fastProvider.TranslateStream(draftCtx, text, opts,
				func(delta string, accumulated string, isDone bool) error {
					sc.mu.Lock()
					if ver < sc.latestReceivedVer || (tid > 0 && tid != sc.activeTurnID) {
						sc.mu.Unlock()
						return draftCtx.Err()
					}
					if isDone {
						sc.latestReceivedVer = ver
						sc.lastDraftTranslation = accumulated
					}
					sc.mu.Unlock()

					sc.emitCaption(flow, tid, nil, text, accumulated, false)
					return nil
				})
		}(version, textToTranslate, turnID)
	})
}

func splitAtStrongPunctuation(text string) (string, string, bool) {
	puncts := []string{"。", "！", "？", ". ", "! ", "? "}
	lastIdx := -1
	pLen := 0
	for _, p := range puncts {
		idx := strings.LastIndex(text, p)
		if idx > lastIdx {
			lastIdx = idx
			pLen = len(p)
		}
	}
	if lastIdx > 0 && lastIdx+pLen < len(text) {
		prefix := strings.TrimSpace(text[:lastIdx+pLen])
		remainder := strings.TrimSpace(text[lastIdx+pLen:])
		if len(prefix) > 0 && len(remainder) > 0 {
			return prefix, remainder, true
		}
	}
	return "", "", false
}
