package lazyledger

import (
	"errors"
	"fmt"
	"strconv"
	"testing"

	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	"github.com/lazyledger/optimint/da"
	"github.com/lazyledger/optimint/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfiguration(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		input    []byte
		err      error
		expected Config
	}{
		{"empty config", []byte(""), errors.New("unknown keyring backend "), Config{}},
		{"with namespace id", []byte("NamespaceID = [3, 2, 1]\nBackend = 'test'"), nil, Config{NamespaceID: []byte{0x03, 0x02, 0x01}, Backend: "test"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert := assert.New(t)
			ll := &LazyLedger{}
			err := ll.Init(c.input, nil)

			if c.err != nil {
				assert.EqualError(err, c.err.Error())
			} else {
				assert.NoError(err)
				assert.Equal(c.expected, ll.config)
			}

		})
	}
}

func TestSubmission(t *testing.T) {
	t.Skip("unfinished test/implementation")
	assert := assert.New(t)
	require := require.New(t)
	block := &types.Block{Header: types.Header{
		Height: 1,
	}}

	ll := &LazyLedger{}
	kr := generateKeyring(t)
	key, err := kr.Key("test-account")
	require.NoError(err)
	conf := testConfig(key)
	err = ll.Init([]byte(conf), nil)
	require.NoError(err)
	ll.keyring = kr

	err = ll.Start()
	require.NoError(err)

	result := ll.SubmitBlock(block)
	assert.Equal("", result.Message)
	assert.Equal(da.StatusSuccess, result.Code)
}

func testConfig(key keyring.Info) string {
	keyStr := ""
	for _, b := range key.GetPubKey().Bytes() {
		keyStr += strconv.Itoa(int(b)) + ", "
	}
	conf := fmt.Sprintf(`PubKey=[%s]
	Backend = 'test'
	From = '%s'
	KeyringAccName = 'test-account'
	RPCAddress = '127.0.0.1:9090'
	NamespaceID = [3, 2, 1, 0, 3, 2, 1, 0]
	GasLimit = 100000
	FeeAmount = 1
	ChainID = 'test'

	`, keyStr, key.GetAddress().String())
	return conf
}

func Test_buildTx(t *testing.T) {
	kr := generateKeyring(t)
	key, err := kr.Key("test-account")
	require.NoError(t, err)
	conf := testConfig(key)
	ll := LazyLedger{}
	err = ll.Init([]byte(conf), nil)
	require.NoError(t, err)
	ll.keyring = kr

	key, err = ll.keyring.Key("test-account")
	require.NoError(t, err)

	block := &types.Block{Header: types.Header{
		Height: 1,
	}}

	msg, err := ll.preparePayForMessage(block)
	require.NoError(t, err)

	signedTx, err := ll.buildTx(msg, 0, 0)
	require.NoError(t, err)

	sigs, err := signedTx.GetSignaturesV2()
	require.NoError(t, err)

	signerData := authsigning.SignerData{
		ChainID:       "test",
		AccountNumber: 0,
		Sequence:      0,
	}

	// Generated Protobuf-encoded bytes.
	require.NoError(t, err)
	err = authsigning.VerifySignature(key.GetPubKey(), signerData, sigs[0].Data, ll.encCfg.TxConfig.SignModeHandler(), signedTx)
	require.NoError(t, err)
}

func generateKeyring(t *testing.T, accts ...string) keyring.Keyring {
	t.Helper()
	kb := keyring.NewInMemory()

	for _, acc := range accts {
		_, _, err := kb.NewMnemonic(acc, keyring.English, "", hd.Secp256k1)
		if err != nil {
			t.Error(err)
		}
	}

	_, err := kb.NewAccount(testAccName, testMnemo, "1234", "", hd.Secp256k1)
	if err != nil {
		panic(err)
	}

	return kb
}

const (
	testMnemo   = `ramp soldier connect gadget domain mutual staff unusual first midnight iron good deputy wage vehicle mutual spike unlock rocket delay hundred script tumble choose`
	testAccName = "test-account"
)
