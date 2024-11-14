package amqp

import "github.com/google/uuid"

func GenerateUUID() string {
	id := ""
	for id == "" {
		value, err := uuid.NewRandom()
		if err != nil {
			continue
		}
		id = value.String()
	}
	return id
}
