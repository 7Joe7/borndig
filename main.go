package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gordonklaus/portaudio"
	"github.com/haguro/elevenlabs-go"
	"github.com/hajimehoshi/go-mp3"
	"github.com/hajimehoshi/oto/v2"
	"github.com/sashabaranov/go-openai"
)

func main() {
	elevenLabsKey := os.Getenv("ELEVENLABS_API_KEY")
	if elevenLabsKey == "" {
		log.Fatal("❌ Please set ELEVENLABS_API_KEY environment variable")
	}

	groqKey := os.Getenv("GROQ_API_KEY")
	if groqKey == "" {
		fmt.Println("❌  GROQ_API_KEY not set → bonus LLM disabled (echo only)")
	}

	client := elevenlabs.NewClient(context.Background(), elevenLabsKey, 60*time.Second)
	voiceID := "21m00Tcm4TlvDq8ikWAM"

	// ==================== GROQ CLIENT ====================
	var groqClient *openai.Client
	if groqKey != "" {
		cfg := openai.DefaultConfig(groqKey)
		cfg.BaseURL = "https://api.groq.com/openai/v1"
		groqClient = openai.NewClientWithConfig(cfg)
		fmt.Println("✅ Groq LLM enabled (bonus active)")
	} else {
		fmt.Println("⚠️  GROQ_API_KEY not set → using simple echo (no LLM bonus)")
	}

	fmt.Println("🎤 Voice Echo App – ElevenLabs STT + ElevenLabs TTS")
	fmt.Println("Speak → press ENTER when finished\n")

	for {
		fmt.Print("Ready → Speak now: ")
		audioWAV := recordMicrophone()
		if len(audioWAV) == 0 {
			continue
		}

		text, err := transcribeWithElevenLabs(elevenLabsKey, audioWAV)
		if err != nil || text == "" {
			fmt.Println("❌ Could not understand. Try again.")
			continue
		}

		fmt.Printf("You said: %s\n", text)

		// ==================== BONUS LLM REACTION ====================
		text = getLLMReaction(groqClient, text)

		speakWithEleven(client, voiceID, text)
	}
}

// ==================== RECORDING ====================
func recordMicrophone() []byte {
	portaudio.Initialize()
	defer portaudio.Terminate()

	const (
		sampleRate = 16000
		channels   = 1
		frames     = 512
		maxSec     = 12
	)

	var audioData []int16
	done := make(chan bool, 1)

	callback := func(in []int16, out []int16, timeInfo portaudio.StreamCallbackTimeInfo, flags portaudio.StreamCallbackFlags) {
		audioData = append(audioData, in...)
	}

	stream, err := portaudio.OpenDefaultStream(channels, 0, float64(sampleRate), frames, callback)
	if err != nil {
		log.Printf("PortAudio open error: %v", err)
		return nil
	}
	defer stream.Close()

	if err := stream.Start(); err != nil {
		log.Printf("Stream start error: %v", err)
		return nil
	}
	defer stream.Stop()

	fmt.Println("(Recording... press ENTER when finished speaking)")

	go func() {
		fmt.Scanln()
		done <- true
	}()

	select {
	case <-done:
		fmt.Println("Recording stopped.")
	case <-time.After(maxSec * time.Second):
		fmt.Println("Recording timeout.")
	}

	return createWAV(audioData, sampleRate, channels)
}

func createWAV(data []int16, sampleRate, channels int) []byte {
	if len(data) == 0 {
		return nil
	}
	var buf bytes.Buffer

	buf.WriteString("RIFF")
	binary.Write(&buf, binary.LittleEndian, uint32(36+len(data)*2))
	buf.WriteString("WAVE")

	buf.WriteString("fmt ")
	binary.Write(&buf, binary.LittleEndian, uint32(16))
	binary.Write(&buf, binary.LittleEndian, uint16(1))
	binary.Write(&buf, binary.LittleEndian, uint16(channels))
	binary.Write(&buf, binary.LittleEndian, uint32(sampleRate))
	binary.Write(&buf, binary.LittleEndian, uint32(sampleRate*channels*2))
	binary.Write(&buf, binary.LittleEndian, uint16(channels*2))
	binary.Write(&buf, binary.LittleEndian, uint16(16))

	buf.WriteString("data")
	binary.Write(&buf, binary.LittleEndian, uint32(len(data)*2))

	for _, sample := range data {
		binary.Write(&buf, binary.LittleEndian, sample)
	}

	return buf.Bytes()
}

// ==================== STT – ElevenLabs Scribe v2 ====================
func transcribeWithElevenLabs(apiKey string, audioWAV []byte) (string, error) {
	url := "https://api.elevenlabs.io/v1/speech-to-text"

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "recording.wav")
	io.Copy(part, bytes.NewReader(audioWAV))
	writer.WriteField("model_id", "scribe_v2")
	writer.Close()

	req, _ := http.NewRequest("POST", url, body)
	req.Header.Set("xi-api-key", apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	clientHTTP := &http.Client{Timeout: 30 * time.Second}
	resp, err := clientHTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			fmt.Println("Reading body failed: %v", err)
		}
		return "", fmt.Errorf("STT failed %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		Text string `json:"text"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.Text, nil
}

// ==================== BONUS: LLM Reaction (Groq) ====================
func getLLMReaction(client *openai.Client, userText string) string {
	if client == nil {
		return userText // fallback to echo
	}

	resp, err := client.CreateChatCompletion(context.Background(), openai.ChatCompletionRequest{
		Model: "llama-3.1-8b-instant", // fast & free
		Messages: []openai.ChatCompletionMessage{
			{Role: "system", Content: "You are a friendly, concise, and fun assistant. Reply naturally in the same language as the user."},
			{Role: "user", Content: userText},
		},
		MaxTokens: 150,
	})

	if err != nil {
		log.Printf("LLM error (using echo instead): %v", err)
		return userText
	}

	return strings.TrimSpace(resp.Choices[0].Message.Content)
}

// ==================== TTS – ElevenLabs (fixed for your library version) ====================
func speakWithEleven(client *elevenlabs.Client, voiceID, text string) {
	req := elevenlabs.TextToSpeechRequest{
		Text:    text,
		ModelID: "eleven_turbo_v2_5", // fast & good quality
	}

	audio, err := client.TextToSpeech(voiceID, req) // ← this is the correct call
	if err != nil {
		log.Printf("TTS error: %v", err)
		return
	}

	playAudio(audio)
}

func playAudio(mp3Data []byte) {
	dec, err := mp3.NewDecoder(bytes.NewReader(mp3Data))
	if err != nil {
		log.Printf("MP3 decode error: %v", err)
		return
	}

	otoCtx, ready, err := oto.NewContext(44100, 2, 2)
	if err != nil {
		log.Printf("Audio context error: %v", err)
		return
	}
	<-ready

	player := otoCtx.NewPlayer(dec)
	defer player.Close()

	player.Play()
	for player.IsPlaying() {
		time.Sleep(100 * time.Millisecond)
	}
}
