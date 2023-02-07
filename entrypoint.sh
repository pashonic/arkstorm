#!/bin/bash
 
if [ -v AWS_S3_BUCKET_CONFIG_FILE ];
then
    aws s3 cp $AWS_S3_BUCKET_CONFIG_FILE ./config.toml
fi

if [ -v AWS_S3_CREDS_BUCKET ];
then
    aws s3 cp --recursive $AWS_S3_CREDS_BUCKET ./
fi

./arkstorm config.toml


