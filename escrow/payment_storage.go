package escrow

import (
	"reflect"
)

// PaymentStorage is a storage for PaymentChannelData by
// PaymentChannelKey based on TypedAtomicStorage implementation
type PaymentStorage struct {
	delegate TypedAtomicStorage
}

// NewPaymentStorage returns new instance of PaymentStorage
// implementation
func NewPaymentStorage(atomicStorage AtomicStorage) *PaymentStorage {
	return &PaymentStorage{
		delegate: &TypedAtomicStorageImpl{
			atomicStorage: &PrefixedAtomicStorage{
				delegate:  atomicStorage,
				keyPrefix: "/payment/storage",
			},
			keySerializer:     serialize,
			valueSerializer:   serialize,
			valueDeserializer: deserialize,
			valueType:         reflect.TypeOf(Payment{}),
		},
	}
}

func (storage *PaymentStorage) GetAll() (states []*Payment, err error) {
	values, err := storage.delegate.GetAll()
	if err != nil {
		return
	}

	return values.([]*Payment), nil
}

func (storage *PaymentStorage) Put(payment *Payment) (err error) {
	return storage.delegate.Put(payment.ID(), payment)
}

func (storage *PaymentStorage) Delete(payment *Payment) (err error) {
	return storage.delegate.Delete(payment.ID())
}
