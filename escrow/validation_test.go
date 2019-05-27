package escrow

import (
	"bytes"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/singnet/snet-daemon/blockchain"
)

func ChannelPaymentValidatorMock() *ChannelPaymentValidator {
	return &ChannelPaymentValidator{
		currentBlock:               func() (*big.Int, error) { return big.NewInt(99), nil },
		paymentExpirationThreshold: func() *big.Int { return big.NewInt(0) },
	}
}

func SignTestPayment(payment *Payment, privateKey *ecdsa.PrivateKey) {
	message := bytes.Join([][]byte{
		payment.MpeContractAddress.Bytes(),
		bigIntToBytes(payment.ChannelID),
		bigIntToBytes(payment.ChannelNonce),
		bigIntToBytes(payment.Amount),
	}, nil)

	payment.Signature = getSignature(message, privateKey)
}

func getSignature(message []byte, privateKey *ecdsa.PrivateKey) (signature []byte) {
	hash := crypto.Keccak256(
		blockchain.HashPrefix32Bytes,
		crypto.Keccak256(message),
	)

	signature, err := crypto.Sign(hash, privateKey)
	if err != nil {
		panic(fmt.Sprintf("Cannot sign test message: %v", err))
	}

	return signature
}

func GenerateTestPrivateKey() (privateKey *ecdsa.PrivateKey) {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		panic(fmt.Sprintf("Cannot generate private key for test: %v", err))
	}
	return
}

type ValidationTestSuite struct {
	suite.Suite

	senderAddress      common.Address
	signerPrivateKey   *ecdsa.PrivateKey
	signerAddress      common.Address
	recipientAddress   common.Address
	mpeContractAddress common.Address

	validator ChannelPaymentValidator
}

func TestValidationTestSuite(t *testing.T) {
	suite.Run(t, new(ValidationTestSuite))
}

func (suite *ValidationTestSuite) SetupSuite() {
	suite.senderAddress = crypto.PubkeyToAddress(GenerateTestPrivateKey().PublicKey)
	suite.signerPrivateKey = GenerateTestPrivateKey()
	suite.signerAddress = crypto.PubkeyToAddress(suite.signerPrivateKey.PublicKey)
	suite.recipientAddress = crypto.PubkeyToAddress(GenerateTestPrivateKey().PublicKey)
	suite.mpeContractAddress = blockchain.HexToAddress("0xf25186b5081ff5ce73482ad761db0eb0d25abfbf")

	suite.validator = ChannelPaymentValidator{
		currentBlock:               func() (*big.Int, error) { return big.NewInt(99), nil },
		paymentExpirationThreshold: func() *big.Int { return big.NewInt(0) },
	}
}

func (suite *ValidationTestSuite) payment() *Payment {
	payment := &Payment{
		Amount:             big.NewInt(12345),
		ChannelID:          big.NewInt(42),
		ChannelNonce:       big.NewInt(3),
		MpeContractAddress: suite.mpeContractAddress,
	}
	SignTestPayment(payment, suite.signerPrivateKey)
	return payment
}

func (suite *ValidationTestSuite) channel() *PaymentChannelData {
	return &PaymentChannelData{
		ChannelID:        big.NewInt(42),
		Nonce:            big.NewInt(3),
		Sender:           suite.senderAddress,
		Recipient:        suite.recipientAddress,
		GroupID:          [32]byte{123},
		FullAmount:       big.NewInt(12345),
		Expiration:       big.NewInt(100),
		Signer:           suite.signerAddress,
		AuthorizedAmount: big.NewInt(12300),
		Signature:        nil,
	}
}

func (suite *ValidationTestSuite) TestPaymentIsValid() {
	payment := suite.payment()
	channel := suite.channel()

	err := suite.validator.Validate(payment, channel)

	assert.Nil(suite.T(), err, "Unexpected error: %v", err)
}

func (suite *ValidationTestSuite) TestValidatePaymentChannelNonce() {
	payment := suite.payment()
	payment.ChannelNonce = big.NewInt(2)
	SignTestPayment(payment, suite.signerPrivateKey)
	channel := suite.channel()
	channel.Nonce = big.NewInt(3)

	err := suite.validator.Validate(payment, channel)

	assert.Equal(suite.T(), NewPaymentError(IncorrectNonce, "incorrect payment channel nonce, latest: 3, sent: 2"), err)
}

func (suite *ValidationTestSuite) TestValidatePaymentIncorrectSignatureLength() {
	payment := suite.payment()
	payment.Signature = blockchain.HexToBytes("0x0000")

	err := suite.validator.Validate(payment, suite.channel())

	assert.Equal(suite.T(), NewPaymentError(Unauthenticated, "payment signature is not valid"), err)
}

func (suite *ValidationTestSuite) TestValidatePaymentIncorrectSignatureChecksum() {
	payment := suite.payment()
	payment.Signature = blockchain.HexToBytes("0xa4d2ae6f3edd1f7fe77e4f6f78ba18d62e6093bcae01ef86d5de902d33662fa372011287ea2d8d8436d9db8a366f43480678df25453b484c67f80941ef2c05ef21")

	err := suite.validator.Validate(payment, suite.channel())

	assert.Equal(suite.T(), NewPaymentError(Unauthenticated, "payment signature is not valid"), err)
}

func (suite *ValidationTestSuite) TestValidatePaymentIncorrectSigner() {
	payment := suite.payment()
	payment.Signature = blockchain.HexToBytes("0xa4d2ae6f3edd1f7fe77e4f6f78ba18d62e6093bcae01ef86d5de902d33662fa372011287ea2d8d8436d9db8a366f43480678df25453b484c67f80941ef2c05ef01")

	err := suite.validator.Validate(payment, suite.channel())

	assert.Equal(suite.T(), NewPaymentError(Unauthenticated, "payment is not signed by channel signer"), err)
}

func (suite *ValidationTestSuite) TestValidatePaymentChannelCannotGetCurrentBlock() {
	validator := &ChannelPaymentValidator{
		currentBlock: func() (*big.Int, error) { return nil, errors.New("blockchain error") },
	}

	err := validator.Validate(suite.payment(), suite.channel())

	assert.Equal(suite.T(), NewPaymentError(Internal, "cannot determine current block"), err)
}

func (suite *ValidationTestSuite) TestValidatePaymentExpiredChannel() {
	validator := &ChannelPaymentValidator{
		currentBlock:               func() (*big.Int, error) { return big.NewInt(99), nil },
		paymentExpirationThreshold: func() *big.Int { return big.NewInt(0) },
	}
	channel := suite.channel()
	channel.Expiration = big.NewInt(99)

	err := validator.Validate(suite.payment(), channel)

	assert.Equal(suite.T(), NewPaymentError(Unauthenticated, "payment channel is near to be expired, expiration time: 99, current block: 99, expiration threshold: 0"), err)
}

func (suite *ValidationTestSuite) TestValidatePaymentChannelExpirationThreshold() {
	validator := &ChannelPaymentValidator{
		currentBlock:               func() (*big.Int, error) { return big.NewInt(98), nil },
		paymentExpirationThreshold: func() *big.Int { return big.NewInt(1) },
	}
	channel := suite.channel()
	channel.Expiration = big.NewInt(99)

	err := validator.Validate(suite.payment(), channel)

	assert.Equal(suite.T(), NewPaymentError(Unauthenticated, "payment channel is near to be expired, expiration time: 99, current block: 98, expiration threshold: 1"), err)
}

func (suite *ValidationTestSuite) TestValidatePaymentAmountIsTooBig() {
	payment := suite.payment()
	payment.Amount = big.NewInt(12346)
	SignTestPayment(payment, suite.signerPrivateKey)
	channel := suite.channel()
	channel.FullAmount = big.NewInt(12345)

	err := suite.validator.Validate(payment, suite.channel())

	assert.Equal(suite.T(), NewPaymentError(Unauthenticated, "not enough tokens on payment channel, channel amount: 12345, payment amount: 12346"), err)
}

func (suite *ValidationTestSuite) TestGetPublicKeyFromPayment() {
	payment := Payment{
		MpeContractAddress: suite.mpeContractAddress,
		ChannelID:          big.NewInt(1789),
		ChannelNonce:       big.NewInt(1917),
		Amount:             big.NewInt(31415),
		// message hash: 04cc38aa4a27976907ef7382182bc549957dc9d2e21eb73651ad6588d5cd4d8f
		Signature: blockchain.HexToBytes("0xa4d2ae6f3edd1f7fe77e4f6f78ba18d62e6093bcae01ef86d5de902d33662fa372011287ea2d8d8436d9db8a366f43480678df25453b484c67f80941ef2c05ef01"),
	}

	address, err := getSignerAddressFromPayment(&payment)

	assert.Nil(suite.T(), err, "Unexpected error: %v", err)
	assert.Equal(suite.T(), blockchain.HexToAddress("0xc5fdf4076b8f3a5357c5e395ab970b5b54098fef"), *address)
}

func (suite *ValidationTestSuite) TestGetPublicKeyFromPayment2() {
	payment := Payment{
		MpeContractAddress: blockchain.HexToAddress("0x39ee715b50e78a920120c1ded58b1a47f571ab75"),
		ChannelID:          big.NewInt(1789),
		ChannelNonce:       big.NewInt(1917),
		Amount:             big.NewInt(31415),
		Signature:          blockchain.HexToBytes("0xde4e998341307b036e460b1cc1593ddefe2e9ea261bd6c3d75967b29b2c3d0a24969b4a32b099ae2eded90bbc213ad0a159a66af6d55be7e04f724ffa52ce3cc1b"),
	}

	address, err := getSignerAddressFromPayment(&payment)

	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), blockchain.HexToAddress("0x592E3C0f3B038A0D673F19a18a773F993d4b2610"), *address)
}
