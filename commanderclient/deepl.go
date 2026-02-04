package commanderclient

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// DeepL API constants
const (
	DeepLDefaultBaseURL = "https://api.deepl.com/v2"
	DeepLDefaultTimeout = 10 * time.Second
)

// DeepLSourceLang represents supported source languages
type DeepLSourceLang string

const (
	DeepLSourceDE DeepLSourceLang = "DE" // German
	DeepLSourceEN DeepLSourceLang = "EN" // English
	DeepLSourceFR DeepLSourceLang = "FR" // French
	DeepLSourceES DeepLSourceLang = "ES" // Spanish
	DeepLSourceIT DeepLSourceLang = "IT" // Italian
	DeepLSourceNL DeepLSourceLang = "NL" // Dutch
	DeepLSourcePL DeepLSourceLang = "PL" // Polish
	DeepLSourcePT DeepLSourceLang = "PT" // Portuguese
	DeepLSourceRU DeepLSourceLang = "RU" // Russian
	DeepLSourceJA DeepLSourceLang = "JA" // Japanese
	DeepLSourceZH DeepLSourceLang = "ZH" // Chinese
)

// DeepLTargetLang represents supported target languages
type DeepLTargetLang string

const (
	DeepLTargetDE   DeepLTargetLang = "DE"    // German
	DeepLTargetENGB DeepLTargetLang = "EN-GB" // English (British)
	DeepLTargetENUS DeepLTargetLang = "EN-US" // English (American)
	DeepLTargetFR   DeepLTargetLang = "FR"    // French
	DeepLTargetES   DeepLTargetLang = "ES"    // Spanish
	DeepLTargetIT   DeepLTargetLang = "IT"    // Italian
	DeepLTargetNL   DeepLTargetLang = "NL"    // Dutch
	DeepLTargetPL   DeepLTargetLang = "PL"    // Polish
	DeepLTargetPTBR DeepLTargetLang = "PT-BR" // Portuguese (Brazilian)
	DeepLTargetPTPT DeepLTargetLang = "PT-PT" // Portuguese (European)
	DeepLTargetRU   DeepLTargetLang = "RU"    // Russian
	DeepLTargetJA   DeepLTargetLang = "JA"    // Japanese
	DeepLTargetZH   DeepLTargetLang = "ZH"    // Chinese (simplified)
)

// DeepLSplitSentences controls sentence splitting behavior
type DeepLSplitSentences string

const (
	DeepLSplitSentencesNone       DeepLSplitSentences = "0"          // No splitting
	DeepLSplitSentencesDefault    DeepLSplitSentences = "1"          // Split on punctuation and newlines
	DeepLSplitSentencesNoNewlines DeepLSplitSentences = "nonewlines" // Split on punctuation only
)

// DeepLFormality controls translation formality
type DeepLFormality string

const (
	DeepLFormalityDefault    DeepLFormality = "default"
	DeepLFormalityMore       DeepLFormality = "more"
	DeepLFormalityLess       DeepLFormality = "less"
	DeepLFormalityPreferMore DeepLFormality = "prefer_more"
	DeepLFormalityPreferLess DeepLFormality = "prefer_less"
)

// DeepLModelType controls the translation model
type DeepLModelType string

const (
	DeepLModelTypeQualityOptimized       DeepLModelType = "quality_optimized"
	DeepLModelTypePreferQualityOptimized DeepLModelType = "prefer_quality_optimized"
	DeepLModelTypeLatencyOptimized       DeepLModelType = "latency_optimized"
)

// DeepLClient is the DeepL API client
type DeepLClient struct {
	httpClient *http.Client
	baseURL    string
	authKey    string
}

// DeepLClientOption configures a DeepLClient
type DeepLClientOption func(*DeepLClient)

// WithDeepLBaseURL sets a custom base URL (useful for testing or proxies)
func WithDeepLBaseURL(baseURL string) DeepLClientOption {
	return func(c *DeepLClient) {
		c.baseURL = baseURL
	}
}

// WithDeepLTimeout sets a custom HTTP timeout
func WithDeepLTimeout(timeout time.Duration) DeepLClientOption {
	return func(c *DeepLClient) {
		c.httpClient.Timeout = timeout
	}
}

// NewDeepLClient creates a new DeepL API client
func NewDeepLClient(authKey string, options ...DeepLClientOption) *DeepLClient {
	client := &DeepLClient{
		httpClient: &http.Client{
			Timeout: DeepLDefaultTimeout,
		},
		baseURL: DeepLDefaultBaseURL,
		authKey: authKey,
	}

	for _, option := range options {
		option(client)
	}

	return client
}

// DeepLTranslateRequest represents a translation request
type DeepLTranslateRequest struct {
	Text               []string            `json:"text"`                            // Required: Text to translate
	SourceLang         DeepLSourceLang     `json:"source_lang,omitempty"`           // Optional: Source language
	TargetLang         DeepLTargetLang     `json:"target_lang"`                     // Required: Target language
	Context            string              `json:"context,omitempty"`               // Optional: Context for translation
	ShowBilledChars    *bool               `json:"show_billed_characters,omitempty"`
	SplitSentences     DeepLSplitSentences `json:"split_sentences,omitempty"`
	PreserveFormatting *bool               `json:"preserve_formatting,omitempty"`
	Formality          DeepLFormality      `json:"formality,omitempty"`
	ModelType          DeepLModelType      `json:"model_type,omitempty"`
	GlossaryID         string              `json:"glossary_id,omitempty"`
}

// DeepLTranslation represents a single translation result
type DeepLTranslation struct {
	DetectedSourceLanguage string         `json:"detected_source_language"`
	Text                   string         `json:"text"`
	BilledCharacters       int            `json:"billed_characters,omitempty"`
	ModelTypeUsed          DeepLModelType `json:"model_type_used,omitempty"`
}

// DeepLTranslateResponse represents the API response
type DeepLTranslateResponse struct {
	Translations []DeepLTranslation `json:"translations"`
}

// DeepLAPIError represents an API error
type DeepLAPIError struct {
	StatusCode int
	Message    string
}

func (e *DeepLAPIError) Error() string {
	return fmt.Sprintf("DeepL API error: %d - %s", e.StatusCode, e.Message)
}

// Translate sends a translation request to the DeepL API
func (c *DeepLClient) Translate(req DeepLTranslateRequest) (*DeepLTranslateResponse, error) {
	if len(req.Text) == 0 {
		return nil, errors.New("text is required")
	}

	if req.TargetLang == "" {
		return nil, errors.New("target_lang is required")
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	endpoint, err := url.JoinPath(c.baseURL, "translate")
	if err != nil {
		return nil, fmt.Errorf("failed to create API URL: %w", err)
	}

	httpReq, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	httpReq.Header.Set("Authorization", "DeepL-Auth-Key "+c.authKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &DeepLAPIError{
			StatusCode: resp.StatusCode,
			Message:    string(body),
		}
	}

	var result DeepLTranslateResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

// TranslateText is a convenience function for translating a single text string.
// Returns the translated text and the number of billed characters.
func (c *DeepLClient) TranslateText(text string, targetLang DeepLTargetLang, sourceLang DeepLSourceLang) (string, int, error) {
	showBilled := true
	req := DeepLTranslateRequest{
		Text:            []string{text},
		TargetLang:      targetLang,
		ShowBilledChars: &showBilled,
	}

	if sourceLang != "" {
		req.SourceLang = sourceLang
	}

	resp, err := c.Translate(req)
	if err != nil {
		return "", 0, err
	}

	if len(resp.Translations) == 0 {
		return "", 0, errors.New("no translation returned")
	}

	return resp.Translations[0].Text, resp.Translations[0].BilledCharacters, nil
}

// DeepLTranslator wraps a DeepLClient for use with TranslateField and TranslateFieldBatch.
// It provides convenience methods that implement TranslateFunc and TranslateBatchFunc.
type DeepLTranslator struct {
	Client     *DeepLClient
	SourceLang DeepLSourceLang
	TargetLang DeepLTargetLang
}

// NewDeepLTranslator creates a new DeepLTranslator with the given client and language settings.
func NewDeepLTranslator(client *DeepLClient, sourceLang DeepLSourceLang, targetLang DeepLTargetLang) *DeepLTranslator {
	return &DeepLTranslator{
		Client:     client,
		SourceLang: sourceLang,
		TargetLang: targetLang,
	}
}

// Translate implements TranslateFunc for single text translation.
// Returns the translated text and the number of billed characters.
func (d *DeepLTranslator) Translate(text string) (string, int, error) {
	return d.Client.TranslateText(text, d.TargetLang, d.SourceLang)
}

// TranslateBatch implements TranslateBatchFunc for batch translation.
// Returns the translated texts and the total number of billed characters.
func (d *DeepLTranslator) TranslateBatch(texts []string) ([]string, int, error) {
	showBilled := true
	resp, err := d.Client.Translate(DeepLTranslateRequest{
		Text:            texts,
		SourceLang:      d.SourceLang,
		TargetLang:      d.TargetLang,
		ShowBilledChars: &showBilled,
	})
	if err != nil {
		return nil, 0, err
	}

	results := make([]string, len(resp.Translations))
	totalBilled := 0
	for i, t := range resp.Translations {
		results[i] = t.Text
		totalBilled += t.BilledCharacters
	}
	return results, totalBilled, nil
}

// AsTranslateFunc returns the Translate method as a TranslateFunc.
// Use this when calling TranslateField.
func (d *DeepLTranslator) AsTranslateFunc() TranslateFunc {
	return d.Translate
}

// AsTranslateBatchFunc returns the TranslateBatch method as a TranslateBatchFunc.
// Use this when calling TranslateFieldBatch.
func (d *DeepLTranslator) AsTranslateBatchFunc() TranslateBatchFunc {
	return d.TranslateBatch
}
