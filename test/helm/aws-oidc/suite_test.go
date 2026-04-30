/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package aws_oidc_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAwsOidc(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AWS OIDC Suite")
}
