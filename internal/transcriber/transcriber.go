package transcriber

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const maxTranscriptRunes = 4000

// Backend identifies which transcription engine to use.
type Backend int

const (
	BackendNone Backend = iota
	BackendOpenAIWhisper
	BackendWhisperCpp
)

func (b Backend) String() string {
	switch b {
	case BackendOpenAIWhisper:
		return "openai-whisper"
	case BackendWhisperCpp:
		return "whisper.cpp"
	default:
		return "none"
	}
}

// Detect resolves a backend hint to a concrete backend by probing $PATH.
// hint: "" or "auto" → prefer whisper.cpp, fall back to openai-whisper.
func Detect(hint string) (Backend, error) {
	switch strings.ToLower(strings.TrimSpace(hint)) {
	case "openai-whisper":
		if _, err := exec.LookPath("whisper"); err != nil {
			return BackendNone, fmt.Errorf("openai-whisper requested but `whisper` not found in $PATH (install: pip install openai-whisper)")
		}
		return BackendOpenAIWhisper, nil
	case "whisper.cpp", "whisper-cpp":
		if _, err := exec.LookPath("whisper-cli"); err != nil {
			return BackendNone, fmt.Errorf("whisper.cpp requested but `whisper-cli` not found in $PATH (install: brew install whisper-cpp)")
		}
		return BackendWhisperCpp, nil
	case "", "auto":
		if _, err := exec.LookPath("whisper-cli"); err == nil {
			return BackendWhisperCpp, nil
		}
		if _, err := exec.LookPath("whisper"); err == nil {
			return BackendOpenAIWhisper, nil
		}
		return BackendNone, fmt.Errorf("no transcription backend found in $PATH (install: brew install whisper-cpp, or pip install openai-whisper)")
	default:
		return BackendNone, fmt.Errorf("unknown whisper backend %q (valid: auto, openai-whisper, whisper.cpp)", hint)
	}
}

// Transcribe runs the selected backend on audioPath and returns the transcribed text.
// backendHint comes from config; "" / "auto" picks the best available backend.
// model is backend-specific: openai-whisper takes a name (e.g. "turbo");
// whisper.cpp takes a path to ggml-*.bin or a short name resolved via standard dirs.
func Transcribe(ctx context.Context, audioPath, backendHint, model string) (string, error) {
	backend, err := Detect(backendHint)
	if err != nil {
		return "", err
	}
	switch backend {
	case BackendOpenAIWhisper:
		return transcribeOpenAI(ctx, audioPath, model)
	case BackendWhisperCpp:
		return transcribeWhisperCpp(ctx, audioPath, model)
	default:
		return "", fmt.Errorf("no transcription backend configured")
	}
}

func transcribeOpenAI(ctx context.Context, audioPath, model string) (string, error) {
	dir := filepath.Dir(audioPath)
	base := strings.TrimSuffix(filepath.Base(audioPath), filepath.Ext(audioPath))
	outFile := filepath.Join(dir, base+".txt")

	args := []string{audioPath,
		"--output_format", "txt",
		"--output_dir", dir,
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	cmd := exec.CommandContext(ctx, "whisper", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("whisper failed: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return readTranscript(outFile)
}

func transcribeWhisperCpp(ctx context.Context, audioPath, model string) (string, error) {
	if model == "" {
		return "", fmt.Errorf("whisper.cpp requires --whisper-model (path to ggml-*.bin or short name like \"base\")")
	}
	modelPath, err := resolveWhisperCppModel(model)
	if err != nil {
		return "", err
	}

	wavPath, cleanup, err := ensureWAV(ctx, audioPath)
	if err != nil {
		return "", err
	}
	defer cleanup()

	dir := filepath.Dir(wavPath)
	base := strings.TrimSuffix(filepath.Base(wavPath), filepath.Ext(wavPath))
	outBase := filepath.Join(dir, base)
	outFile := outBase + ".txt"

	cmd := exec.CommandContext(ctx, "whisper-cli",
		"-m", modelPath,
		"-f", wavPath,
		"-otxt",
		"-of", outBase,
		"-nt",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("whisper-cli failed: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return readTranscript(outFile)
}

func readTranscript(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read whisper output file: %w", err)
	}
	text := strings.TrimSpace(string(data))
	runes := []rune(text)
	if len(runes) > maxTranscriptRunes {
		runes = runes[:maxTranscriptRunes]
	}
	return string(runes), nil
}

// resolveWhisperCppModel turns a model arg into an absolute path to a ggml-*.bin file.
// Accepts an existing path (.bin or containing /), or a short name resolved
// against $WHISPER_CPP_MODELS and ~/.cache/whisper-cpp/.
func resolveWhisperCppModel(model string) (string, error) {
	if strings.HasSuffix(model, ".bin") || strings.ContainsRune(model, '/') {
		if _, err := os.Stat(model); err != nil {
			return "", fmt.Errorf("whisper model file not found: %s", model)
		}
		abs, err := filepath.Abs(model)
		if err != nil {
			return model, nil
		}
		return abs, nil
	}

	var candidates []string
	if dir := os.Getenv("WHISPER_CPP_MODELS"); dir != "" {
		candidates = append(candidates, filepath.Join(dir, "ggml-"+model+".bin"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates,
			filepath.Join(home, ".cache", "whisper-cpp", "ggml-"+model+".bin"),
		)
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf(
		"whisper.cpp model %q not found; tried: %s\n"+
			"download with: mkdir -p ~/.cache/whisper-cpp && "+
			"curl -L -o ~/.cache/whisper-cpp/ggml-%s.bin "+
			"https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-%s.bin",
		model, strings.Join(candidates, ", "), model, model,
	)
}

// ensureWAV converts audioPath to a 16kHz mono WAV via ffmpeg if it isn't already a .wav.
// Returns (path, cleanup). cleanup deletes the temp WAV when the original wasn't already WAV.
func ensureWAV(ctx context.Context, audioPath string) (string, func(), error) {
	noop := func() {}
	if strings.EqualFold(filepath.Ext(audioPath), ".wav") {
		return audioPath, noop, nil
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return "", noop, fmt.Errorf("ffmpeg required to convert audio for whisper.cpp (install: brew install ffmpeg)")
	}
	dir := filepath.Dir(audioPath)
	base := strings.TrimSuffix(filepath.Base(audioPath), filepath.Ext(audioPath))
	wavPath := filepath.Join(dir, base+".wav")
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-loglevel", "error",
		"-i", audioPath,
		"-ar", "16000",
		"-ac", "1",
		wavPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", noop, fmt.Errorf("ffmpeg conversion failed: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	cleanup := func() { _ = os.Remove(wavPath) }
	return wavPath, cleanup, nil
}
