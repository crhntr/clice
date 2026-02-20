package clice

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAppendCellID(t *testing.T) {
	t.Run("append", func(t *testing.T) {
		const prefix = "peachy"
		buf := make([]byte, 0, 19+len(prefix))
		buf = append(buf, prefix...)
		for exp := range 10 {
			n := powInt(10, exp)
			t.Run(strconv.Itoa(n), func(t *testing.T) {
				var out []byte
				allocCount := testing.AllocsPerRun(1, func() {
					out, _ = ID{Column: n, Row: n}.AppendText(buf)
				})
				require.Zero(t, allocCount)
				require.Equal(t, prefix, string(buf[:len(prefix)]), "don't edit the prefix")
				t.Logf("%q has len(%d) and cap(%d)", string(out), len(out), cap(out))
			})
		}
	})

	t.Run("make", func(t *testing.T) {
		for exp := range 10 {
			n := powInt(10, exp)
			t.Run(strconv.Itoa(n), func(t *testing.T) {
				var out []byte
				allocCount := testing.AllocsPerRun(1, func() {
					out, _ = ID{Column: n, Row: n}.AppendText(nil)
				})
				require.Equal(t, 1.0, allocCount)
				require.Equal(t, len(out), cap(out))
				t.Logf("%q has len(%d) and cap(%d)", string(out), len(out), cap(out))
			})
		}
	})

}
