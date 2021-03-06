// Copyright 2020 The Kube-burner Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package measurements

import (
	"github.com/cloud-bulldozer/kube-burner/log"
	"github.com/cloud-bulldozer/kube-burner/pkg/config"
	"github.com/cloud-bulldozer/kube-burner/pkg/indexers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type measurementFactory struct {
	jobConfig   *config.Job
	clientSet   *kubernetes.Clientset
	restConfig  *rest.Config
	createFuncs map[string]measurement
	indexer     *indexers.Indexer
	uuid        string
}

type measurement interface {
	start()
	stop() (int, error)
	setConfig(config.Measurement)
}

var factory measurementFactory
var measurementMap = make(map[string]measurement)
var kubeburnerCfg *config.GlobalConfig = &config.ConfigSpec.GlobalConfig

// NewMeasurementFactory initializes the measurement facture
func NewMeasurementFactory(restConfig *rest.Config, uuid string, indexer *indexers.Indexer) {
	log.Info("📈 Creating measurement factory")
	clientSet := kubernetes.NewForConfigOrDie(restConfig)
	factory = measurementFactory{
		clientSet:   clientSet,
		restConfig:  restConfig,
		createFuncs: make(map[string]measurement),
		indexer:     indexer,
		uuid:        uuid,
	}
	for _, measurement := range kubeburnerCfg.Measurements {
		if measurementFunc, exists := measurementMap[measurement.Name]; exists {
			factory.register(measurement, measurementFunc)
		} else {
			log.Warnf("Measurement not found: %s", measurement.Name)
		}
	}
}

func (mf *measurementFactory) register(measure config.Measurement, measurementFunc measurement) {
	if _, exists := mf.createFuncs[measure.Name]; exists {
		log.Warnf("Measurement already registered: %s", measure.Name)
	} else {
		measurementFunc.setConfig(measure)
		mf.createFuncs[measure.Name] = measurementFunc
		log.Infof("Registered measurement: %s", measure.Name)
	}
}

func SetJobConfig(jobConfig *config.Job) {
	factory.jobConfig = jobConfig
}

// Start starts registered measurements
func Start() {
	for _, measurement := range factory.createFuncs {
		go measurement.start()
	}
}

// Stop stops registered measurements
func Stop() int {
	var err error
	var r, rc int
	for name, measurement := range factory.createFuncs {
		log.Infof("Stopping measurement: %s", name)
		if r, err = measurement.stop(); err != nil {
			log.Errorf("Error stopping measurement %s: %s", name, err)
		}
		if r != 0 {
			rc = r
		}
	}
	return rc
}
