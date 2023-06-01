package utils

import "github.com/aws/amazon-vpc-cni-k8s/test/framework"

func GetTestImage(image string) string {
	f := framework.New(framework.GlobalOptions)
	return f.Options.TestImageRegistry + "/" + image
}
