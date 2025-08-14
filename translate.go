package main

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

type Language struct {
	Code string `json:"code"`
	Name string `json:"name"`
	Flag string `json:"flag"`
	File string `json:"file"`
}

type LanguageConfig struct {
	SupportedLanguages []Language `json:"supported_languages"`
	DefaultLanguage    string     `json:"default_language"`
}

type Translation struct {
	TokenMap           map[string]string
	CurrentLanguage    string
	SupportedLanguages []Language
	currentIndex       int
}

//go:embed languages
var languageFS embed.FS

// TODO: Load this from an embedded file
func setupTranslation() *Translation {
	trans := Translation{
		TokenMap: make(map[string]string),
	}

	// Load language configuration
	config, err := loadLanguageConfig()
	if err != nil {
		fmt.Printf("Error loading language config: %v\n", err)
		// Fallback to default
		trans.SupportedLanguages = []Language{
			{Code: "en", Name: "English", Flag: "🇬🇧", File: "en.json"},
		}
		trans.CurrentLanguage = "en"
		trans.currentIndex = 0
		return &trans
	}

	trans.SupportedLanguages = config.SupportedLanguages
	trans.CurrentLanguage = config.DefaultLanguage

	// Find index of default language
	for i, lang := range trans.SupportedLanguages {
		if lang.Code == trans.CurrentLanguage {
			trans.currentIndex = i
			break
		}
	}

	// Load default language translations
	err = trans.loadLanguageTokens(trans.CurrentLanguage)
	if err != nil {
		fmt.Printf("Error loading default language tokens: %v\n", err)
	}

	return &trans
}

func loadLanguageConfig() (*LanguageConfig, error) {
	// Try to load from file system first (for development)
	if data, err := os.ReadFile("languages/languages.json"); err == nil {
		var config LanguageConfig
		if err := json.Unmarshal(data, &config); err == nil {
			return &config, nil
		}
	}

	// Fallback to embedded file system
	data, err := languageFS.ReadFile("languages/languages.json")
	if err != nil {
		return nil, fmt.Errorf("failed to read languages.json: %w", err)
	}

	var config LanguageConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse languages.json: %w", err)
	}

	return &config, nil
}

func (trans *Translation) loadLanguageTokens(langCode string) error {
	var langFile string
	for _, lang := range trans.SupportedLanguages {
		if lang.Code == langCode {
			langFile = lang.File
			break
		}
	}

	if langFile == "" {
		return fmt.Errorf("language file not found for code: %s", langCode)
	}

	// Try to load from file system first (for development)
	if data, err := os.ReadFile("languages/" + langFile); err == nil {
		var tokens map[string]string
		if err := json.Unmarshal(data, &tokens); err == nil {
			trans.TokenMap = tokens
			return nil
		}
	}

	// Fallback to embedded file system
	data, err := languageFS.ReadFile("languages/" + langFile)
	if err != nil {
		return fmt.Errorf("failed to read language file %s: %w", langFile, err)
	}

	var tokens map[string]string
	if err := json.Unmarshal(data, &tokens); err != nil {
		return fmt.Errorf("failed to parse language file %s: %w", langFile, err)
	}

	trans.TokenMap = tokens
	return nil
}

func (trans *Translation) getLanguages() []Language {
	return trans.SupportedLanguages
}

func (trans *Translation) setLanguage(lang string) error {
	for i, v := range trans.SupportedLanguages {
		if v.Code == lang {
			trans.currentIndex = i
			trans.CurrentLanguage = lang
			// Load new language tokens
			err := trans.loadLanguageTokens(lang)
			if err != nil {
				return fmt.Errorf("failed to load language tokens for %s: %w", lang, err)
			}
			return nil
		}
	}
	return errors.New("Language not supported")
}

func (trans *Translation) getCurrentLanguage() Language {
	if trans.currentIndex < len(trans.SupportedLanguages) {
		return trans.SupportedLanguages[trans.currentIndex]
	}
	return Language{Code: "en", Name: "English", Flag: "🇬🇧"}
}

func (trans *Translation) lookupToken(token string) string {
	s, avail := trans.TokenMap[token]
	if !avail {
		return token // Return the token itself as fallback
	}
	return s
}
