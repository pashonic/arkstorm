package sendsns

import (
	"os"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sns"
)

const (
	env_sns_arn = "YOUTUBE_UPLOAD_ALERT_SNS_ARN"
)

func SendSNS(subject string, message string) error {
	snsArn := os.Getenv(env_sns_arn)
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
	return err
}
