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
	LogDebug("TRANSLATE", "Setting up translation system", "")
	
	trans := Translation{
		TokenMap: make(map[string]string),
	}

	// Load language configuration
	LogDebug("TRANSLATE", "Loading language configuration", "")
	config, err := loadLanguageConfig()
	if err != nil {
		LogWarn("TRANSLATE", "Failed to load language config, using fallback", err.Error())
		fmt.Printf("Error loading language config: %v\n", err)
		// Fallback to default
		trans.SupportedLanguages = []Language{
			{Code: "en", Name: "English", Flag: "🇬🇧", File: "en.json"},
		}
		trans.CurrentLanguage = "en"
		trans.currentIndex = 0
		LogInfo("TRANSLATE", "Translation system initialized with fallback", "Default language: en")
		return &trans
	}

	trans.SupportedLanguages = config.SupportedLanguages
	trans.CurrentLanguage = config.DefaultLanguage
	LogDebug("TRANSLATE", "Language configuration loaded", fmt.Sprintf("Supported languages: %d, Default: %s", len(trans.SupportedLanguages), trans.CurrentLanguage))

	// Find index of default language
	for i, lang := range trans.SupportedLanguages {
		if lang.Code == trans.CurrentLanguage {
			trans.currentIndex = i
			LogDebug("TRANSLATE", "Default language index found", fmt.Sprintf("Language: %s, Index: %d", lang.Code, i))
			break
		}
	}

	// Load default language translations
	LogDebug("TRANSLATE", "Loading default language tokens", fmt.Sprintf("Language: %s", trans.CurrentLanguage))
	err = trans.loadLanguageTokens(trans.CurrentLanguage)
	if err != nil {
		LogWarn("TRANSLATE", "Failed to load default language tokens", err.Error())
		fmt.Printf("Error loading default language tokens: %v\n", err)
	} else {
		LogInfo("TRANSLATE", "Default language tokens loaded", fmt.Sprintf("Language: %s, Tokens: %d", trans.CurrentLanguage, len(trans.TokenMap)))
	}

	LogInfo("TRANSLATE", "Translation system initialized successfully", fmt.Sprintf("Languages: %d, Current: %s", len(trans.SupportedLanguages), trans.CurrentLanguage))
	return &trans
}

func loadLanguageConfig() (*LanguageConfig, error) {
	LogDebug("TRANSLATE", "Loading language configuration", "")
	
	// Try to load from file system first (for development)
	LogDebug("TRANSLATE", "Attempting to load from file system", "languages/languages.json")
	if data, err := os.ReadFile("languages/languages.json"); err == nil {
		LogDebug("TRANSLATE", "Language config file found on filesystem", fmt.Sprintf("Size: %d bytes", len(data)))
		var config LanguageConfig
		if err := json.Unmarshal(data, &config); err == nil {
			LogDebug("TRANSLATE", "Language config parsed from filesystem", fmt.Sprintf("Languages: %d", len(config.SupportedLanguages)))
			return &config, nil
		} else {
			LogWarn("TRANSLATE", "Failed to parse language config from filesystem", err.Error())
		}
	} else {
		LogDebug("TRANSLATE", "Language config not found on filesystem", err.Error())
	}

	// Fallback to embedded file system
	LogDebug("TRANSLATE", "Loading from embedded file system", "languages/languages.json")
	data, err := languageFS.ReadFile("languages/languages.json")
	if err != nil {
		LogError("TRANSLATE", "Failed to read embedded languages.json", err.Error())
		return nil, fmt.Errorf("failed to read languages.json: %w", err)
	}

	LogDebug("TRANSLATE", "Language config loaded from embedded filesystem", fmt.Sprintf("Size: %d bytes", len(data)))
	var config LanguageConfig
	if err := json.Unmarshal(data, &config); err != nil {
		LogError("TRANSLATE", "Failed to parse embedded languages.json", err.Error())
		return nil, fmt.Errorf("failed to parse languages.json: %w", err)
	}

	LogInfo("TRANSLATE", "Language configuration loaded successfully", fmt.Sprintf("Languages: %d, Default: %s", len(config.SupportedLanguages), config.DefaultLanguage))
	return &config, nil
}

func (trans *Translation) loadLanguageTokens(langCode string) error {
	LogDebug("TRANSLATE", "Loading language tokens", fmt.Sprintf("Language code: %s", langCode))
	
	var langFile string
	for _, lang := range trans.SupportedLanguages {
		if lang.Code == langCode {
			langFile = lang.File
			LogDebug("TRANSLATE", "Language file found", fmt.Sprintf("Code: %s, File: %s", langCode, langFile))
			break
		}
	}

	if langFile == "" {
		LogError("TRANSLATE", "Language file not found", fmt.Sprintf("Code: %s", langCode))
		return fmt.Errorf("language file not found for code: %s", langCode)
	}

	// Try to load from file system first (for development)
	filePath := "languages/" + langFile
	LogDebug("TRANSLATE", "Attempting to load from filesystem", fmt.Sprintf("File: %s", filePath))
	if data, err := os.ReadFile(filePath); err == nil {
		LogDebug("TRANSLATE", "Language file found on filesystem", fmt.Sprintf("File: %s, Size: %d bytes", langFile, len(data)))
		var tokens map[string]string
		if err := json.Unmarshal(data, &tokens); err == nil {
			trans.TokenMap = tokens
			LogDebug("TRANSLATE", "Language tokens loaded from filesystem", fmt.Sprintf("File: %s, Tokens: %d", langFile, len(tokens)))
			return nil
		} else {
			LogWarn("TRANSLATE", "Failed to parse language file from filesystem", fmt.Sprintf("File: %s, Error: %s", langFile, err.Error()))
		}
	} else {
		LogDebug("TRANSLATE", "Language file not found on filesystem", fmt.Sprintf("File: %s, Error: %s", langFile, err.Error()))
	}

	// Fallback to embedded file system
	LogDebug("TRANSLATE", "Loading from embedded file system", fmt.Sprintf("File: %s", langFile))
	data, err := languageFS.ReadFile("languages/" + langFile)
	if err != nil {
		LogError("TRANSLATE", "Failed to read embedded language file", fmt.Sprintf("File: %s, Error: %s", langFile, err.Error()))
		return fmt.Errorf("failed to read language file %s: %w", langFile, err)
	}

	LogDebug("TRANSLATE", "Language file loaded from embedded filesystem", fmt.Sprintf("File: %s, Size: %d bytes", langFile, len(data)))
	var tokens map[string]string
	if err := json.Unmarshal(data, &tokens); err != nil {
		LogError("TRANSLATE", "Failed to parse embedded language file", fmt.Sprintf("File: %s, Error: %s", langFile, err.Error()))
		return fmt.Errorf("failed to parse language file %s: %w", langFile, err)
	}

	trans.TokenMap = tokens
	LogInfo("TRANSLATE", "Language tokens loaded successfully", fmt.Sprintf("File: %s, Tokens: %d", langFile, len(tokens)))
	return nil
}

func (trans *Translation) getLanguages() []Language {
	LogDebug("TRANSLATE", "Getting supported languages", fmt.Sprintf("Count: %d", len(trans.SupportedLanguages)))
	return trans.SupportedLanguages
}

func (trans *Translation) setLanguage(lang string) error {
	LogDebug("TRANSLATE", "Setting language", fmt.Sprintf("Language: %s", lang))
	
	for i, v := range trans.SupportedLanguages {
		if v.Code == lang {
			LogDebug("TRANSLATE", "Language found in supported languages", fmt.Sprintf("Language: %s, Index: %d", lang, i))
			trans.currentIndex = i
			trans.CurrentLanguage = lang
			// Load new language tokens
			LogDebug("TRANSLATE", "Loading tokens for new language", fmt.Sprintf("Language: %s", lang))
			err := trans.loadLanguageTokens(lang)
			if err != nil {
				LogError("TRANSLATE", "Failed to load language tokens", fmt.Sprintf("Language: %s, Error: %s", lang, err.Error()))
				return fmt.Errorf("failed to load language tokens for %s: %w", lang, err)
			}
			LogInfo("TRANSLATE", "Language changed successfully", fmt.Sprintf("Language: %s, Tokens: %d", lang, len(trans.TokenMap)))
			return nil
		}
	}
	LogWarn("TRANSLATE", "Language not supported", fmt.Sprintf("Language: %s", lang))
	return errors.New("Language not supported")
}

func (trans *Translation) getCurrentLanguage() Language {
	LogDebug("TRANSLATE", "Getting current language", fmt.Sprintf("Index: %d, Total: %d", trans.currentIndex, len(trans.SupportedLanguages)))
	
	if trans.currentIndex < len(trans.SupportedLanguages) {
		currentLang := trans.SupportedLanguages[trans.currentIndex]
		LogDebug("TRANSLATE", "Current language retrieved", fmt.Sprintf("Language: %s (%s)", currentLang.Name, currentLang.Code))
		return currentLang
	}
	LogWarn("TRANSLATE", "Current language index out of bounds, using fallback", fmt.Sprintf("Index: %d, Total: %d", trans.currentIndex, len(trans.SupportedLanguages)))
	return Language{Code: "en", Name: "English", Flag: "🇬🇧"}
}

func (trans *Translation) lookupToken(token string) string {
	LogDebug("TRANSLATE", "Looking up token", fmt.Sprintf("Token: %s", token))
	
	s, avail := trans.TokenMap[token]
	if !avail {
		LogDebug("TRANSLATE", "Token not found, using fallback", fmt.Sprintf("Token: %s", token))
		return token // Return the token itself as fallback
	}
	LogDebug("TRANSLATE", "Token found", fmt.Sprintf("Token: %s, Translation: %s", token, s))
	return s
}
