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
	"context"
	"fmt"
	"os"
	"path"
	"sync"
	"time"

	"github.com/cloud-bulldozer/kube-burner/log"
	"github.com/cloud-bulldozer/kube-burner/pkg/config"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/kubectl/pkg/scheme"

	"k8s.io/apimachinery/pkg/labels"
)

type pprof struct {
	directory   string
	config      config.Measurement
	stopChannel chan bool
}

func init() {
	measurementMap["pprof"] = &pprof{}
}

func (p *pprof) setConfig(cfg config.Measurement) {
	p.directory = "pprof"
	if cfg.PProfDirectory != "" {
		p.directory = cfg.PProfDirectory
	}
	p.config = cfg
}

func (p *pprof) start() {
	err := os.MkdirAll(p.directory, 0744)
	if err != nil {
		log.Fatalf("Error creating pprof directory: %s", err)
	}
	p.stopChannel = make(chan bool)
	p.getPProf()
	go func() {
		defer close(p.stopChannel)
		ticker := time.NewTicker(p.config.PProfInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				p.getPProf()
			case <-p.stopChannel:
				return
			}
		}
	}()
}

func getPods(target config.PProftarget) []corev1.Pod {
	labelSelector := labels.Set(target.LabelSelector).String()
	podList, err := factory.clientSet.CoreV1().Pods(target.Namespace).List(context.TODO(), v1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		log.Errorf("Error found listing pods labeled with %s: %s", labelSelector, err)
	}
	return podList.Items
}

func (p *pprof) getPProf() {
	var wg sync.WaitGroup
	var command []string
	for _, target := range p.config.PProfTargets {
		log.Infof("Collecting %s pprof", target.Name)
		podList := getPods(target)
		for _, pod := range podList {
			wg.Add(1)
			go func(target config.PProftarget, pod corev1.Pod) {
				defer wg.Done()
				pprofFile := fmt.Sprintf("%s-%s-%d.pprof", target.Name, pod.Name, time.Now().Unix())
				f, err := os.Create(path.Join(p.directory, pprofFile))
				if err != nil {
					log.Errorf("Error creating pprof file %s: %s", pprofFile, err)
					return
				}
				defer f.Close()
				if target.BearerToken != "" {
					command = []string{"curl", "-sSLkH", fmt.Sprintf("Authorization:  Bearer %s", target.BearerToken), target.URL}
				} else {
					command = []string{"curl", "-sSLkH", target.URL}
				}
				req := factory.clientSet.CoreV1().
					RESTClient().
					Post().
					Resource("pods").
					Name(pod.Name).
					Namespace(pod.Namespace).
					SubResource("exec")
				req.VersionedParams(&corev1.PodExecOptions{
					Command:   command,
					Container: pod.Spec.Containers[0].Name,
					Stdin:     false,
					Stderr:    true,
					Stdout:    true,
				}, scheme.ParameterCodec)
				exec, err := remotecommand.NewSPDYExecutor(factory.restConfig, "POST", req.URL())
				if err != nil {
					log.Errorf("Failed to execute pprof command on %s: %s", target.Name, err)
				}
				err = exec.Stream(remotecommand.StreamOptions{
					Stdin:  nil,
					Stdout: f,
					Stderr: f,
				})
				if err != nil {
					log.Errorf("Failed to get results from %s: %s", target.Name, err)
				}
			}(target, pod)
		}
	}
	wg.Wait()
}

func (p *pprof) stop() (int, error) {
	p.stopChannel <- true
	return 0, nil
}
