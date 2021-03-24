package annotations_test

import (
	"github.com/haproxytech/kubernetes-ingress/controller/annotations"
	"github.com/haproxytech/kubernetes-ingress/controller/store"
)

func (suite *AnnotationSuite) TestMaxconnUpdate() {
	test := store.StringW{Value: "200"}
	a := annotations.NewGlobalMaxconn("", suite.client)
	if suite.NoError(a.Parse(test, true)) {
		suite.NoError(a.Update())
		result, _ := suite.client.GlobalWriteConfig("global", "maxconn")
		suite.Equal("maxconn 200", result)
	}
}

func (suite *AnnotationSuite) TestMaxconnFail() {
	test := store.StringW{Value: "garbage"}
	a := annotations.NewGlobalMaxconn("", suite.client)
	err := a.Parse(test, true)
	suite.T().Log(err)
	suite.Error(err)
}
