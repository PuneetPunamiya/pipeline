package message

// MessageLengthError indicate the length of termination message of container is beyond 4096 which is the max length read by kubenates
type MessageLengthError string

const (
	errTooLong MessageLengthError = "Termination message is above max allowed size 4096, caused by large task result."
)

func (e MessageLengthError) Error() string {
	return string(e)
}
