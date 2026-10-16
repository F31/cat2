package llm

import (
	"testing"

	"catcube/internal/ports/suite"
)

// 端口契约：LLM/OCR/ASR（开发态实现）。
func TestLLMContract(t *testing.T) { suite.LLM(t, NewDevClient()) }
func TestOCRContract(t *testing.T) { suite.OCR(t, NewDevOCR()) }
func TestASRContract(t *testing.T) { suite.ASR(t, NewDevASR()) }
