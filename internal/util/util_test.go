package util_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michalkurzeja/godi/v2/internal/util"
)

func TestFuncName(t *testing.T) {
	tests := []struct {
		name      string
		fn        any
		wantFull  string
		wantShort string
	}{
		{
			name:      "myFunc",
			fn:        myFunc,
			wantFull:  "github.com/michalkurzeja/godi/v2/internal/util_test.myFunc",
			wantShort: "myFunc",
		},
		{
			name:      "nil",
			fn:        nil,
			wantFull:  "<not a func>",
			wantShort: "<not a func>",
		},
		{
			name:      "anonymous func",
			fn:        func() int { return 0 },
			wantFull:  "github.com/michalkurzeja/godi/v2/internal/util_test.TestFuncName.func1",
			wantShort: "func1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.wantFull, util.FuncName(reflect.ValueOf(tt.fn)))
			require.Equal(t, tt.wantShort, util.FuncNameShort(reflect.ValueOf(tt.fn)))
		})
	}
}

func myFunc() {}
