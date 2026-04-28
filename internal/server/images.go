package server

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"dragonqr/internal/game"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

const stationImageModel = openai.ImageModel("gpt-image-2")

type organizerCode struct {
	game.Code
	URL         string
	ImageURL    string
	HasImage    bool
	GenerateURL string
}

func (s *Server) handleGenerateMissingImages(w http.ResponseWriter, r *http.Request) {
	count, err := s.generateMissingImages(r.Context())
	if err != nil {
		s.render(w, "organizer.html", s.organizerView(r, "", err.Error()))
		return
	}
	s.render(w, "organizer.html", s.organizerView(r, fmt.Sprintf("Generated %d missing station images.", count), ""))
}

func (s *Server) handleGenerateCodeImage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	code, ok := s.quest.Code(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if s.codeImageExists(code) {
		s.render(w, "organizer.html", s.organizerView(r, "That station already has an image.", ""))
		return
	}
	if err := s.generateCodeImage(r.Context(), code); err != nil {
		s.render(w, "organizer.html", s.organizerView(r, "", err.Error()))
		return
	}
	s.render(w, "organizer.html", s.organizerView(r, fmt.Sprintf("Generated image for %s.", code.ID), ""))
}

func (s *Server) generateMissingImages(ctx context.Context) (int, error) {
	count := 0
	for _, code := range s.quest.Codes {
		if s.codeImageExists(code) {
			continue
		}
		if err := s.generateCodeImage(ctx, code); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (s *Server) generateCodeImage(ctx context.Context, code game.Code) error {
	if strings.TrimSpace(s.cfg.OpenAIAPIKey) == "" {
		return fmt.Errorf("OPENAI_API_KEY is required to generate images")
	}
	imagePath, err := s.codeImagePath(code)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(imagePath), 0o755); err != nil {
		return fmt.Errorf("create generated image directory: %w", err)
	}

	client := openai.NewClient(option.WithAPIKey(s.cfg.OpenAIAPIKey))
	resp, err := client.Images.Generate(ctx, openai.ImageGenerateParams{
		Prompt:       stationImagePrompt(s.quest, code),
		Model:        stationImageModel,
		N:            openai.Int(1),
		OutputFormat: openai.ImageGenerateParamsOutputFormatWebP,
		Quality:      openai.ImageGenerateParamsQualityMedium,
		Size:         openai.ImageGenerateParamsSize1024x1024,
	})
	if err != nil {
		return fmt.Errorf("generate image for %s: %w", code.ID, err)
	}
	if len(resp.Data) == 0 {
		return fmt.Errorf("generate image for %s: no image returned", code.ID)
	}
	imageBody := strings.TrimSpace(resp.Data[0].B64JSON)
	if imageBody == "" {
		return fmt.Errorf("generate image for %s: empty image returned", code.ID)
	}
	imageBytes, err := base64.StdEncoding.DecodeString(imageBody)
	if err != nil {
		return fmt.Errorf("decode generated image for %s: %w", code.ID, err)
	}
	if err := os.WriteFile(imagePath, imageBytes, 0o644); err != nil {
		return fmt.Errorf("save generated image for %s: %w", code.ID, err)
	}
	return nil
}

func (s *Server) organizerView(r *http.Request, message string, errorMessage string) map[string]any {
	var codes []organizerCode
	for _, code := range s.quest.Codes {
		imageURL := s.codeImageURL(code)
		codes = append(codes, organizerCode{
			Code:        code,
			URL:         s.codeURL(r, code.ID),
			ImageURL:    imageURL,
			HasImage:    imageURL != "",
			GenerateURL: "/organizer/images/generate/" + url.PathEscape(code.ID),
		})
	}
	return map[string]any{
		"Quest":       s.quest,
		"BaseURL":     s.baseURL(r),
		"Codes":       codes,
		"Message":     message,
		"Error":       errorMessage,
		"ImageDir":    s.cfg.GeneratedImageDir,
		"CanGenerate": strings.TrimSpace(s.cfg.OpenAIAPIKey) != "",
	}
}

func (s *Server) codeImageExists(code game.Code) bool {
	imagePath, err := s.codeImagePath(code)
	if err != nil {
		return false
	}
	info, err := os.Stat(imagePath)
	return err == nil && !info.IsDir()
}

func (s *Server) codeImageURL(code game.Code) string {
	if !s.codeImageExists(code) {
		return ""
	}
	return "/static/generated/stations/" + url.PathEscape(code.ID) + ".webp"
}

func (s *Server) codeImagePath(code game.Code) (string, error) {
	if strings.ContainsAny(code.ID, `/\`) {
		return "", fmt.Errorf("code %q cannot be used as an image filename", code.ID)
	}
	return filepath.Join(s.cfg.GeneratedImageDir, code.ID+".webp"), nil
}

func (s *Server) activeCombatURL(r *http.Request, p *game.Player) string {
	if p.Combat == nil {
		return ""
	}
	return s.codeURL(r, p.Combat.CodeID)
}

func stationImagePrompt(q *game.Quest, code game.Code) string {
	if prompt := strings.TrimSpace(code.ImagePrompt); prompt != "" {
		return prompt
	}
	parts := []string{
		"Create a warm children's fantasy illustration for a QR scavenger hunt station.",
		"Use a playful storybook style with clear subject, rich color, and no text, letters, logos, URLs, or QR codes.",
		"Quest: " + q.Title + ".",
		"Station type: " + string(code.Type) + ".",
		"Station title: " + code.Title + ".",
	}
	if strings.TrimSpace(code.Description) != "" {
		parts = append(parts, "Scene: "+code.Description)
	}
	return strings.Join(parts, " ")
}
