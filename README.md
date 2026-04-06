# Voice Echo App + LLM Bonus

A simple local Go application that listens to your microphone, transcribes what you say using **ElevenLabs Scribe (STT)**, and speaks it back using **ElevenLabs TTS**.  
**Bonus feature**: You can enable an intelligent LLM reaction (instead of simple echo) using **Groq** (free tier).

This was created as a home assignment to demonstrate working with modern AI APIs, real-time audio in Go, and clean code.

## Features

- Real-time microphone recording (press Enter to stop)
- Speech-to-Text with ElevenLabs Scribe v2
- Text-to-Speech echo with ElevenLabs Turbo model
- **Bonus**: Optional intelligent LLM reply using Groq (Llama 3.1)
- Minimal dependencies

## Dependencies

### System Dependency
- **PortAudio** – for microphone access

**macOS**:
```bash
brew install portaudio