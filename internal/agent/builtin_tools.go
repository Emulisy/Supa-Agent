package agent

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"time"
	_ "time/tzdata"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/expr-lang/expr"
)

type calculatorInput struct {
	Expression string `json:"expression" jsonschema:"description=Arithmetic expression such as (1+2)*3"`
}

type datetimeInput struct {
	Operation string `json:"operation" jsonschema:"enum=now,enum=add_days,enum=diff"`
	TZ        string `json:"tz,omitempty" jsonschema:"description=IANA time zone. Defaults to Asia/Shanghai"`
	Days      *int   `json:"days,omitempty" jsonschema:"description=Signed number of calendar days required for add_days"`
}

func builtinTool(name string) (tool.BaseTool, error) {
	var t tool.InvokableTool
	var err error
	switch name {
	case "calculator":
		t, err = utils.InferTool(name, "Evaluate a numeric arithmetic expression. Operators include +, -, *, /, %, and ^.", calculate)
	case "datetime":
		t, err = utils.InferTool(name, "Get the current local time, add calendar days, or count calendar days until January 1 of next year (diff).", datetime)
	default:
		return nil, fmt.Errorf("unknown tool %q", name)
	}
	if err != nil {
		return nil, err
	}
	return utils.WrapInvokableToolWithErrorHandler(t, func(_ context.Context, err error) string {
		return "Tool error: " + err.Error()
	}), nil
}

var numericExpression = regexp.MustCompile(`^[0-9.eE+*/%()^\t -]+$`)

func calculate(_ context.Context, input calculatorInput) (string, error) {
	if len(input.Expression) > 512 || !numericExpression.MatchString(input.Expression) {
		return "", errors.New("provide an arithmetic expression of at most 512 characters")
	}
	program, err := expr.Compile(input.Expression, expr.AsFloat64(), expr.DisableAllBuiltins(), expr.MaxNodes(128))
	if err != nil {
		return "", fmt.Errorf("invalid arithmetic: %w", err)
	}
	value, err := expr.Run(program, nil)
	if err != nil {
		return "", fmt.Errorf("invalid arithmetic: %w", err)
	}
	number, ok := value.(float64)
	if !ok || math.IsInf(number, 0) || math.IsNaN(number) {
		return "", errors.New("result must be finite")
	}
	if number == math.Trunc(number) {
		return strconv.FormatFloat(number, 'f', -1, 64), nil
	}
	return strconv.FormatFloat(number, 'f', 6, 64), nil
}

func datetime(_ context.Context, input datetimeInput) (string, error) {
	tz := input.TZ
	if tz == "" {
		tz = "Asia/Shanghai"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil || tz == "Local" {
		return "", errors.New("provide a valid IANA time zone")
	}
	now := time.Now().In(loc)
	switch input.Operation {
	case "now":
		return now.Format(time.RFC3339), nil
	case "add_days":
		if input.Days == nil || *input.Days < -365000 || *input.Days > 365000 {
			return "", errors.New("provide days between -365000 and 365000")
		}
		return now.AddDate(0, 0, *input.Days).Format(time.RFC3339), nil
	case "diff":
		year, month, day := now.Date()
		today := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
		nextYear := time.Date(year+1, 1, 1, 0, 0, 0, 0, time.UTC)
		return fmt.Sprintf("%d days until %d-01-01", nextYear.Sub(today)/(24*time.Hour), year+1), nil
	default:
		return "", errors.New("operation must be now, add_days, or diff")
	}
}
