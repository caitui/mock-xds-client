/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"github.com/caitui/mock-xds-client/pkg/mockclient"
	"github.com/urfave/cli"
	_ "net/http/pprof"
)

var (
	cmdStart = cli.Command{
		Name:  "start",
		Usage: "start mock xds client",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:   "service-cluster, c",
				Usage:  "sidecar service cluster",
				EnvVar: "SERVICE_CLUSTER",
			}, cli.IntFlag{
				Name:   "worker-count, w",
				Usage:  "sidecar worker count",
				EnvVar: "WORKER_COUNT",
			},
		},
		Action: func(c *cli.Context) error {

			serviceCluster := c.String("service-cluster")
			workerCount := c.Int("worker-count")

			// stop signal
			stop := make(chan struct{})
			// watch stop signal
			go WaitSignal(stop)
			// goroutines
			mockclient.MockXdsClients(serviceCluster, workerCount, stop)

			return nil
		},
	}

	cmdStop = cli.Command{
		Name:  "stop",
		Usage: "stop client proxy",
		Flags: []cli.Flag{},
		Action: func(c *cli.Context) (err error) {
			// todo
			return nil
		},
	}

	cmdReload = cli.Command{
		Name:  "reload",
		Usage: "reconfiguration",
		Action: func(c *cli.Context) error {
			return nil
		},
	}
)
