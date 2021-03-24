// Copyright 2019 HAProxy Technologies LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// +build e2e_sequential

package endpoints

import (
	"fmt"
	"net/http"

	"github.com/haproxytech/kubernetes-ingress/deploy/tests/e2e"
)

// For each test we send two times mores requests than replicas available.
// In the end the counter should be 2 for each returned pod-name in request answer.
func (suite *EndpointsSuite) Test_HTTP_Reach() {
	for _, replicas := range []int{4, 8, 2, 0, 3} {
		suite.Run(fmt.Sprintf("%d-replicas", replicas), func() {
			suite.tmplData.Replicas = replicas
			suite.NoError(suite.test.DeployYamlTemplate("config/endpoints.yaml.tmpl", suite.test.GetNS(), suite.tmplData))
			suite.Eventually(func() bool {
				counter := map[string]int{}
				for i := 0; i < replicas*2; i++ {
					func() {
						res, cls, err := suite.client.Do()
						suite.NoError(err)
						defer cls()
						if res.StatusCode == http.StatusOK {
							counter[newReachResponse(suite.T(), res)]++
						}
					}()
				}
				if replicas == 0 && len(counter) > 0 {
					return false
				}
				for _, v := range counter {
					if v != 2 {
						return false
					}
				}
				return true
			}, e2e.WaitDuration, e2e.TickDuration)
		})
	}
}
