package translator

import (
	"strings"
)

// Standard BCP 47 language codes commonly used.
const (
	LangZhCN = "zh-CN"
	LangZhTW = "zh-TW"
	LangEnUS = "en-US"
	LangEnGB = "en-GB"
	LangJaJP = "ja-JP"
	LangKoKR = "ko-KR"
	LangDeDE = "de-DE"
	LangFrFR = "fr-FR"
	LangEsES = "es-ES"
	LangRuRU = "ru-RU"
)

// NormalizeLang normalizes various input formats into standard BCP 47 (e.g. zh -> zh-CN, en -> en-US).
func NormalizeLang(lang string) string {
	cleaned := strings.TrimSpace(lang)
	if cleaned == "" || strings.EqualFold(cleaned, "auto") {
		return "auto"
	}

	// Replace underscores with hyphens (e.g. zh_CN -> zh-CN)
	cleaned = strings.ReplaceAll(cleaned, "_", "-")
	parts := strings.Split(cleaned, "-")
	primary := strings.ToLower(parts[0])

	if len(parts) == 1 {
		switch primary {
		case "zh":
			return LangZhCN
		case "en":
			return LangEnUS
		case "ja":
			return LangJaJP
		case "ko":
			return LangKoKR
		case "de":
			return LangDeDE
		case "fr":
			return LangFrFR
		case "es":
			return LangEsES
		case "ru":
			return LangRuRU
		default:
			return primary
		}
	}

	// Reconstruct with lowercase language and uppercase region (e.g. zh-CN, en-US)
	region := strings.ToUpper(parts[1])
	return primary + "-" + region
}

// ToDeepLTargetLang maps a BCP 47 language code to DeepL API target_lang requirements.
func ToDeepLTargetLang(lang string) string {
	norm := NormalizeLang(lang)
	switch norm {
	case LangZhCN, "zh":
		return "ZH"
	case LangZhTW, "zh-HK", "zh-Hant":
		return "ZH-HANT"
	case LangEnUS, "en":
		return "EN-US"
	case LangEnGB:
		return "EN-GB"
	case "pt-BR":
		return "PT-BR"
	case "pt-PT", "pt":
		return "PT-PT"
	default:
		// DeepL expects 2-letter uppercase for other supported languages (e.g. JA, DE, FR, ES, RU, IT, KO)
		parts := strings.Split(norm, "-")
		return strings.ToUpper(parts[0])
	}
}

// ToDeepLSourceLang maps a BCP 47 language code to DeepL API source_lang requirements (2-letter uppercase).
func ToDeepLSourceLang(lang string) string {
	norm := NormalizeLang(lang)
	if norm == "auto" || norm == "" {
		return ""
	}
	parts := strings.Split(norm, "-")
	return strings.ToUpper(parts[0])
}

// ToOpenAILangDesc maps a BCP 47 language code to an explicit, unambiguous natural language description for LLM prompts.
func ToOpenAILangDesc(lang string) string {
	norm := NormalizeLang(lang)
	switch norm {
	case LangZhCN, "zh":
		return "Simplified Chinese (简体中文, zh-CN)"
	case LangZhTW, "zh-HK", "zh-Hant":
		return "Traditional Chinese (繁體中文, zh-TW)"
	case LangEnUS:
		return "American English (en-US)"
	case LangEnGB:
		return "British English (en-GB)"
	case "en":
		return "English (en-US)"
	case LangJaJP, "ja":
		return "Japanese (日本語, ja-JP)"
	case LangKoKR, "ko":
		return "Korean (한국어, ko-KR)"
	case LangDeDE, "de":
		return "German (Deutsch, de-DE)"
	case LangFrFR, "fr":
		return "French (Français, fr-FR)"
	case LangEsES, "es":
		return "Spanish (Español, es-ES)"
	case LangRuRU, "ru":
		return "Russian (Русский, ru-RU)"
	default:
		return norm
	}
}
