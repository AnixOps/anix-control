package handler

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

// ForwardDiagnosisAnswersTestSuite pins the bytes the legacy diagnosis
// routes answer: the forward node and Ansible machine checks, the node
// statistics, and the forward and tunnel diagnoses. The KernelNodeOps
// diagnose executors (NO-8) share these routes' implementation; the answers
// stay what they were before the routes moved onto it. Only the values that
// change from one call to the next are normalized (normalizeDiagnosisAnswer).
type ForwardDiagnosisAnswersTestSuite struct {
	HandlerTestSuite
}

func TestForwardDiagnosisAnswers(t *testing.T) {
	suite.Run(t, new(ForwardDiagnosisAnswersTestSuite))
}
