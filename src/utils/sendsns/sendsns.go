package sendsns

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sns"
)

func SendSNS(subject string, message string, snsArn string) error {
	if snsArn == "" {
		return nil
	}
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))
	svc := sns.New(sess)
	_, err := svc.Publish(&sns.PublishInput{
		Message:  &message,
		TopicArn: &snsArn,
		Subject:  &subject,
	})
	if err == nil {
		fmt.Println("SNS alert Sent to: " + snsArn)
	}
	return err
}
