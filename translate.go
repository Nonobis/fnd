package main

import (
	"errors"
	"slices"
)

type Translation struct {
	TokenMap           map[string][]string
	CurrentLanguage    string
	SupportedLanguages []string
	currentIndex       int
}

// TODO: Load this from an embedded file
func setupTranslation() *Translation {
	trans := Translation{
		TokenMap: make(map[string][]string),
	}

	trans.SupportedLanguages = []string{"de", "en"}
	trans.CurrentLanguage = "en"
	trans.currentIndex = 1

	trans.TokenMap["header"] = []string{"Frigate Nachrichten Dienst", "Frigate Notification Service"}
	trans.TokenMap["reload"] = []string{"Seite neuladen", "Reload page"}
	trans.TokenMap["overview"] = []string{"Übersicht", "Overview"}
	trans.TokenMap["menu"] = []string{"Menü", "Menu"}
	trans.TokenMap["settings"] = []string{"Einstellungen", "Settings"}
	trans.TokenMap["notifications"] = []string{"Benachrichtigungen", "Notifications"}
	trans.TokenMap["last_notify"] = []string{"Letzte Benachrichtigungen", "Recent notifications"}
	trans.TokenMap["cooldown"] = []string{"Abklingzeit (in Sek)", "Cooldown (in sec)"}
	trans.TokenMap["active_cams"] = []string{"Aktive Kameras", "Active cameras"}
	trans.TokenMap["apply"] = []string{"Übernehmen", "Apply"}
	trans.TokenMap["active"] = []string{"Aktiv", "active"}
	trans.TokenMap["tel_doc"] = []string{
		` <ol>
                <li>Zuerst beim <a href="https://telegram.me/BotFather">BotFather</a> einen neuen Bot erstellen</li>
                <li>Dann den Bot in Telegram starten</li>
                <li>Das Bot Token hier reinkopieren + aktiv anwählen und übernehmen</li>
                <li>Dem Bot /getid schreiben und die Antwort hier in Chat ID reinkopieren und übernehmen</li>
            </ol>`,
		` <ol>
                <li>Create a bot from <a href="https://telegram.me/BotFather">BotFather</a></li>
                <li>Start the bot in telegram</li>
                <li>Copy the bot token and add it here, press apply</li>
                <li>Write /getid to the bot and copy the answer into Chat ID, press apply</li>
            </ol>`,
	}
	trans.TokenMap["confID"] = []string{"Konfigurations ID", "Configuration ID"}
	trans.TokenMap["apprise_doc"] = []string{
		` <ol>
                <li>Zu :7778 wechseln und eine neue Apprise Konfiguration erstellen</li>
                <li>Eine Benachrichtigung in Apprise erstellen und testen</li>
                <li>Die ID hier reinkopieren und übernehmen</li>
            </ol>`,
		` <ol>
                <li>Go to :7778 (or whatever you configured in docker) and create a new Apprise configuration</li>
                <li>Configure notifications in Apprise and test them</li>
                <li>Paste the configuration ID here and apply</li>
            </ol>`,
	}
	trans.TokenMap["camera"] = []string{"Kamera", "camera"}
	trans.TokenMap["object"] = []string{"Objekt", "object"}

	return &trans
}

func (trans *Translation) getLanguages() []string {
	return trans.SupportedLanguages
}

func (trans *Translation) setLanguage(lang string) error {
	if !slices.Contains(trans.SupportedLanguages, lang) {
		return errors.New("Language not supported")
	}

	for i, v := range trans.SupportedLanguages {
		if v == lang {
			trans.currentIndex = i
			break
		}
	}

	trans.CurrentLanguage = lang
	return nil
}

func (trans *Translation) lookupToken(token string) string {
	s, avail := trans.TokenMap[token]
	if !avail {
		return ""
	}

	if len(s) < trans.currentIndex {
		return ""
	}

	return s[trans.currentIndex]
}
