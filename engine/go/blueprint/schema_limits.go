package blueprint

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	maxNodePortCount              = 4096
	maxNodePortID                 = maxNodePortCount - 1
	maxDynamicSequenceOutputCount = 256
	maxFunctionInputCount         = 128
	maxFunctionOutputCount        = 128
)

func validateMaximum(field string, value int, maximum int) error {
	if value > maximum {
		return fmt.Errorf("%s %d exceeds maximum %d", field, value, maximum)
	}
	return nil
}

func validateTotalNodePortCount(inputCount int, outputCount int) error {
	return validateMaximum("total port count", inputCount+outputCount, maxNodePortCount)
}

func validateFunctionPortCounts(inputCount int, outputCount int, inputField string, outputField string) error {
	if err := validateMaximum(inputField, inputCount, maxFunctionInputCount); err != nil {
		return err
	}
	return validateMaximum(outputField, outputCount, maxFunctionOutputCount)
}

func validateNodeConfigLimits(node NodeConfig) error {
	switch node.Class {
	case "FunctionEntry", "SetTimerByFunction":
		return validateFunctionPortCounts(len(node.FunctionInputTypes), 0, "function input count", "function output count")
	case "FunctionReturn":
		return validateFunctionPortCounts(0, len(node.FunctionOutputTypes), "function input count", "function output count")
	case "FunctionCall":
		return validateFunctionPortCounts(len(node.FunctionInputTypes), len(node.FunctionOutputTypes), "function input count", "function output count")
	}

	if !strings.HasPrefix(node.Class, "SequenceDynamic") {
		return nil
	}
	count, err := strconv.Atoi(strings.TrimPrefix(node.Class, "SequenceDynamic"))
	if err != nil || count <= 0 {
		return nil
	}
	return validateMaximum("dynamic output count", count, maxDynamicSequenceOutputCount)
}
