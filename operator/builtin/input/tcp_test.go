package input

import (
	"fmt"
	"math/rand"
	"net"
	"testing"
	"time"

	"github.com/observiq/carbon/entry"
	"github.com/observiq/carbon/internal/testutil"
	"github.com/observiq/carbon/operator"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func tcpInputTest(input []byte, expected []string) func(t *testing.T) {
	return func(t *testing.T) {
		cfg := NewTCPInputConfig("test_id")
		address := newRandListenAddress()
		cfg.ListenAddress = address

		newOperator, err := cfg.Build(testutil.NewBuildContext(t))
		require.NoError(t, err)

		mockOutput := testutil.Operator{}
		tcpInput := newOperator.(*TCPInput)
		tcpInput.InputOperator.OutputOperators = []operator.Operator{&mockOutput}

		entryChan := make(chan *entry.Entry, 1)
		mockOutput.On("Process", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
			entryChan <- args.Get(1).(*entry.Entry)
		}).Return(nil)

		err = tcpInput.Start()
		require.NoError(t, err)
		defer tcpInput.Stop()

		conn, err := net.Dial("tcp", address)
		require.NoError(t, err)
		defer conn.Close()

		_, err = conn.Write(input)
		require.NoError(t, err)

		for _, expectedMessage := range expected {
			select {
			case entry := <-entryChan:
				require.Equal(t, expectedMessage, entry.Record)
			case <-time.After(time.Second):
				require.FailNow(t, "Timed out waiting for message to be written")
			}
		}

		select {
		case entry := <-entryChan:
			require.FailNow(t, "Unexpected entry: %s", entry)
		case <-time.After(100 * time.Millisecond):
			return
		}
	}
}

func TestTcpInput(t *testing.T) {
	t.Run("Simple", tcpInputTest([]byte("message\n"), []string{"message"}))
	t.Run("CarriageReturn", tcpInputTest([]byte("message\r\n"), []string{"message"}))
}

func newRandListenAddress() string {
	port := rand.Int()%16000 + 49152
	return fmt.Sprintf("127.0.0.1:%d", port)
}

func BenchmarkTcpInput(b *testing.B) {
	cfg := NewTCPInputConfig("test_id")
	address := newRandListenAddress()
	cfg.ListenAddress = address

	newOperator, err := cfg.Build(testutil.NewBuildContext(b))
	require.NoError(b, err)

	fakeOutput := testutil.NewFakeOutput(b)
	tcpInput := newOperator.(*TCPInput)
	tcpInput.InputOperator.OutputOperators = []operator.Operator{fakeOutput}

	err = tcpInput.Start()
	require.NoError(b, err)

	done := make(chan struct{})
	go func() {
		conn, err := net.Dial("tcp", address)
		require.NoError(b, err)
		defer tcpInput.Stop()
		defer conn.Close()
		message := []byte("message\n")
		for {
			select {
			case <-done:
				return
			default:
				_, err := conn.Write(message)
				require.NoError(b, err)
			}
		}
	}()

	for i := 0; i < b.N; i++ {
		<-fakeOutput.Received
	}

	defer close(done)
}