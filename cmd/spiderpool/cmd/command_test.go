// Copyright 2022 Authors of spidernet-io
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"reflect"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/plugins/pkg/testutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
	"k8s.io/utils/pointer"

	"github.com/spidernet-io/spiderpool/api/v1/agent/models"
	"github.com/spidernet-io/spiderpool/api/v1/agent/server/restapi/connectivity"
	"github.com/spidernet-io/spiderpool/api/v1/agent/server/restapi/daemonset"
	"github.com/spidernet-io/spiderpool/cmd/spiderpool/cmd"
	"github.com/spidernet-io/spiderpool/pkg/constant"
	"github.com/spidernet-io/spiderpool/pkg/logutils"
	"github.com/spidernet-io/spiderpool/pkg/openapi"
)

const ifName string = "eth0"
const nsPath string = "/some/where"
const containerID string = "dummy"
const CNITimeoutSec = 120
const CNILogFilePath = "/tmp/spiderpool.log"

const (
	healthCheckRoute = "/v1/ipam/healthy"
	ipamReqRoute     = "/v1/ipam/ip"
)

const CNIVersion010 = "0.1.0"
const CNIVersion020 = "0.2.0"

var cniVersion string
var args *skel.CmdArgs
var netConf cmd.NetConf
var sockPath string

var addChan, delChan chan struct{}

type ConfigWorkableSets struct {
	// decide the IPAM plugin configuration is all good
	isPreConfigGood bool
	// decide the spiderpool agent is able to respond route 'healthy'
	isHealthy bool
	// decide the spiderpool agent is able to assign IP
	isPostIPAM bool
	// decide the spiderpool agent is able to release IP
	isDeleteIPAM bool
}

var _ = Describe("spiderpool plugin", Label("unitest", "ipam_plugin_test"), func() {
	BeforeEach(func() {
		// generate one temp unix file.
		tempDir := GinkgoT().TempDir()
		sockPath = tempDir + "/tmp.sock"

		// cleanup the temp unix file at the end.
		DeferCleanup(func() {
			err := os.RemoveAll(sockPath)
			Expect(err).NotTo(HaveOccurred())
			err = os.RemoveAll(CNILogFilePath)
			Expect(err).NotTo(HaveOccurred())
		})

		args = &skel.CmdArgs{
			ContainerID: containerID,
			Netns:       nsPath,
			IfName:      ifName,
		}

		cniVersion = cmd.CniVersion031

		netConf = cmd.NetConf{
			CNIVersion: cniVersion,
			IPAM: cmd.IPAMConfig{
				LogLevel:           logutils.LogDebugLevelStr,
				LogFilePath:        CNILogFilePath,
				IPAMUnixSocketPath: sockPath,
			},
		}

		addChan = make(chan struct{})
		delChan = make(chan struct{})
	})

	Context("mock ipam plugin interacts with agent through unix socket", func() {
		var server *ghttp.Server
		BeforeEach(func() {
			listener, err := net.Listen("unix", sockPath)
			Expect(err).NotTo(HaveOccurred())
			server = ghttp.NewUnstartedServer()
			server.HTTPTestServer.Listener = listener
			server.Start()

			DeferCleanup(func() {
				server.Close()
			})
		})

		DescribeTable("test cmdAdd",
			func(configSets ConfigWorkableSets, cmdArgs func() *skel.CmdArgs, mockServerResponse func() *models.IpamAddResponse, expectResponse func() *current.Result) {
				var ipamPostHandleFunc http.HandlerFunc

				// GET /v1/ipam/healthy
				server.RouteToHandler(http.MethodGet, healthCheckRoute, ghttp.CombineHandlers(getHealthHandleFunc(configSets.isHealthy)))

				// POST /v1/ipam/ip
				if configSets.isPostIPAM {
					// You must pre-define this even if the mockServerResponse is nil!
					// And mockServerResponse is nil only use for bad health check!
					var mockServerResp *models.IpamAddResponse
					if nil != mockServerResponse {
						mockServerResp = mockServerResponse()
					}
					ipamPostHandleFunc = ghttp.RespondWithJSONEncoded(daemonset.PostIpamIpsOKCode, mockServerResp)
				} else {
					ipamPostHandleFunc = ghttp.RespondWithJSONEncoded(daemonset.DeleteIpamIPFailureCode, nil)
				}
				server.RouteToHandler(http.MethodPost, ipamReqRoute, ghttp.CombineHandlers(ipamPostHandleFunc))

				// start client test.
				r, _, err := testutils.CmdAddWithArgs(cmdArgs(), func() error {
					return cmd.CmdAdd(cmdArgs())
				})

				// bad response check
				var expectErr error
				if !configSets.isPreConfigGood {
					Expect(err).To(HaveOccurred())
					By("Expect to match error: " + err.Error())
					return
				} else if !configSets.isHealthy {
					expectErr = cmd.ErrAgentHealthCheck
				} else if !configSets.isPostIPAM {
					expectErr = cmd.ErrPostIPAM
				} else {
					Expect(err).NotTo(HaveOccurred())
				}

				if expectErr != nil {
					Expect(err).To(HaveOccurred())
					Expect(err).Should(MatchError(expectErr))
					return
				}

				addResult, err := current.GetResult(r)
				Expect(err).NotTo(HaveOccurred())

				var expectResp *current.Result
				if nil != expectResponse {
					expectResp = expectResponse()
				} else {
					Fail("You must define expectResp if every route good in CmdAdd situation.")
				}

				// No need to check result.CNIVersion since cni types 100 library would hard code it with "1.0.0"

				// check Result.DNS
				Expect(reflect.DeepEqual(addResult.DNS, expectResp.DNS)).To(Equal(true))

				// check Result.IPs
				Expect(reflect.DeepEqual(addResult.IPs, expectResp.IPs)).To(Equal(true))

				// check Result.Routes
				Expect(reflect.DeepEqual(addResult.Routes, expectResp.Routes))
			},
			Entry("returning an error on bad health check with ADD", ConfigWorkableSets{isPreConfigGood: true, isHealthy: false, isPostIPAM: true}, func() *skel.CmdArgs {
				netConfBytes, err := json.Marshal(netConf)
				Expect(err).NotTo(HaveOccurred())
				args.StdinData = netConfBytes
				return args
			}, nil, nil),
			Entry("returning an error on POST IPAM with ADD", ConfigWorkableSets{isPreConfigGood: true, isHealthy: true, isPostIPAM: false}, func() *skel.CmdArgs {
				netConfBytes, err := json.Marshal(netConf)
				Expect(err).NotTo(HaveOccurred())
				args.StdinData = netConfBytes
				return args
			}, nil, nil),
			Entry(fmt.Sprintf("allocates addresses with ADD in CNI version '%s'", cmd.CniVersion031), ConfigWorkableSets{isPreConfigGood: true, isHealthy: true, isPostIPAM: true}, func() *skel.CmdArgs {
				netConfBytes, err := json.Marshal(netConf)
				Expect(err).NotTo(HaveOccurred())
				args.StdinData = netConfBytes
				return args
			}, func() *models.IpamAddResponse {
				ipamAddResp := &models.IpamAddResponse{
					DNS: &models.DNS{
						Domain:      "local",
						Nameservers: []string{"1.2.3.1"},
						Options:     []string{"somedomain.com"},
						Search:      []string{"foo"},
					},
					Ips: []*models.IPConfig{
						{
							Address: pointer.String("1.2.3.30/24"),
							Gateway: "1.2.3.1",
							Nic:     pointer.String("eth0"),
							Version: pointer.Int64(constant.IPv4),
						},
						{
							Address: pointer.String("fc00:f853:ccd:e793:f::fc/64"),
							Gateway: "fc00:f853:ccd:e793:f::2",
							Nic:     pointer.String("eth0"),
							Version: pointer.Int64(constant.IPv6),
						},
					},
					Routes: []*models.Route{{IfName: pointer.String("eth0"), Dst: pointer.String("15.5.6.0/24"), Gw: pointer.String("1.2.3.2")}},
				}

				return ipamAddResp
			}, func() *current.Result {
				expectResult := new(current.Result)
				// CNIVersion
				expectResult.CNIVersion = cniVersion
				// DNS
				expectResult.DNS = types.DNS{
					Nameservers: []string{"1.2.3.1"},
					Domain:      "local",
					Search:      []string{"foo"},
					Options:     []string{"somedomain.com"},
				}
				// IPs
				expectResult.IPs = []*current.IPConfig{
					{Gateway: net.ParseIP("1.2.3.1"), Address: net.IPNet{IP: net.ParseIP("1.2.3.30"), Mask: net.CIDRMask(24, 32)}},
					{Gateway: net.ParseIP("fc00:f853:ccd:e793:f::2"), Address: net.IPNet{IP: net.ParseIP("fc00:f853:ccd:e793:f::fc"), Mask: net.CIDRMask(64, 128)}},
				}

				// Routes
				_, ipNet, _ := net.ParseCIDR("15.5.6.0/24")
				expectResult.Routes = []*types.Route{{Dst: *ipNet, GW: net.ParseIP("1.2.3.2")}}
				//Interfaces
				expectResult.Interfaces = []*current.Interface{{Name: ifName}}
				return expectResult
			}),
			Entry(fmt.Sprintf("support CNI version '%s'", cmd.CniVersion030), ConfigWorkableSets{isPreConfigGood: true, isHealthy: true, isPostIPAM: true}, func() *skel.CmdArgs {
				netConf.CNIVersion = cmd.CniVersion030
				netConfBytes, err := json.Marshal(netConf)
				Expect(err).NotTo(HaveOccurred())
				args.StdinData = netConfBytes
				return args
			}, func() *models.IpamAddResponse {
				ipamAddResp := &models.IpamAddResponse{
					DNS: &models.DNS{
						Domain:      "local",
						Nameservers: []string{"10.1.0.2"},
						Options:     []string{"domain.com"},
						Search:      []string{"bar"},
					},
					Ips: []*models.IPConfig{
						{
							Address: pointer.String("10.1.0.6/24"),
							Gateway: "10.1.0.2",
							Nic:     pointer.String(ifName),
							Version: pointer.Int64(constant.IPv4),
						},
					},
				}

				return ipamAddResp
			}, func() *current.Result {
				expectResult := new(current.Result)
				// CNIVersion
				expectResult.CNIVersion = cmd.CniVersion030
				// DNS
				expectResult.DNS = types.DNS{
					Nameservers: []string{"10.1.0.2"},
					Domain:      "local",
					Search:      []string{"bar"},
					Options:     []string{"domain.com"},
				}
				// IPs
				expectResult.IPs = []*current.IPConfig{
					{Gateway: net.ParseIP("10.1.0.2"), Address: net.IPNet{IP: net.ParseIP("10.1.0.6"), Mask: net.CIDRMask(24, 32)}},
				}
				//Interfaces
				expectResult.Interfaces = []*current.Interface{{Name: ifName}}
				return expectResult
			}),
			Entry(fmt.Sprintf("support CNI version '%s'", cmd.CniVersion040), ConfigWorkableSets{isPreConfigGood: true, isHealthy: true, isPostIPAM: true}, func() *skel.CmdArgs {
				netConf.CNIVersion = cmd.CniVersion030
				netConfBytes, err := json.Marshal(netConf)
				Expect(err).NotTo(HaveOccurred())
				args.StdinData = netConfBytes
				return args
			}, func() *models.IpamAddResponse {
				ipamAddResp := &models.IpamAddResponse{
					DNS: &models.DNS{},
					Ips: []*models.IPConfig{
						{
							Address: pointer.String("10.1.0.7/24"),
							Nic:     pointer.String(ifName),
							Version: pointer.Int64(constant.IPv4),
						},
					},
				}

				return ipamAddResp
			}, func() *current.Result {
				expectResult := new(current.Result)
				// CNIVersion
				expectResult.CNIVersion = cmd.CniVersion040
				// DNS
				expectResult.DNS = types.DNS{}
				// IPs
				expectResult.IPs = []*current.IPConfig{
					{Address: net.IPNet{IP: net.ParseIP("10.1.0.7"), Mask: net.CIDRMask(24, 32)}},
				}
				//Interfaces
				expectResult.Interfaces = []*current.Interface{{Name: ifName}}
				return expectResult
			}),
			Entry(fmt.Sprintf("support CNI version '%s'", CNIVersion010), ConfigWorkableSets{isPreConfigGood: false, isHealthy: true, isPostIPAM: true}, func() *skel.CmdArgs {
				netConf.CNIVersion = CNIVersion010
				netConfBytes, err := json.Marshal(netConf)
				Expect(err).NotTo(HaveOccurred())
				args.StdinData = netConfBytes
				return args
			}, nil, nil),
			Entry(fmt.Sprintf("support CNI version '%s'", CNIVersion020), ConfigWorkableSets{isPreConfigGood: false, isHealthy: true, isPostIPAM: true}, func() *skel.CmdArgs {
				netConf.CNIVersion = CNIVersion010
				netConfBytes, err := json.Marshal(netConf)
				Expect(err).NotTo(HaveOccurred())
				args.StdinData = netConfBytes
				return args
			}, nil, nil),
		)

		DescribeTable("test cmdDel",
			func(configSets ConfigWorkableSets, cmdArgs func() *skel.CmdArgs) {
				var ipamDeleteHandleFunc http.HandlerFunc

				// GET /v1/ipam/healthy
				server.RouteToHandler(http.MethodGet, healthCheckRoute, ghttp.CombineHandlers(getHealthHandleFunc(configSets.isHealthy)))

				// DELETE /v1/ipam/ip
				if configSets.isDeleteIPAM {
					ipamDeleteHandleFunc = ghttp.RespondWith(daemonset.DeleteIpamIPOKCode, nil)
				} else {
					ipamDeleteHandleFunc = ghttp.RespondWith(daemonset.DeleteIpamIPFailureCode, nil)
				}
				server.RouteToHandler(http.MethodDelete, ipamReqRoute, ghttp.CombineHandlers(ipamDeleteHandleFunc))

				// start client test
				err := testutils.CmdDelWithArgs(cmdArgs(), func() error {
					return cmd.CmdDel(cmdArgs())
				})

				// bad response check
				var expectErr error
				if !configSets.isPreConfigGood {
					Expect(err).To(HaveOccurred())
					By("Expect to match error: " + err.Error())
					return
				} else if !configSets.isHealthy {
					expectErr = cmd.ErrAgentHealthCheck
				} else if !configSets.isDeleteIPAM {
					// the CNI binary cmdDel won't return err
					expectErr = nil
				} else {
					Expect(err).NotTo(HaveOccurred())
				}

				if expectErr != nil {
					Expect(err).To(HaveOccurred())
					Expect(err).Should(MatchError(expectErr))
					return
				}
			},
			Entry("returning an error on bad health check with DEL", ConfigWorkableSets{isPreConfigGood: true, isHealthy: false, isDeleteIPAM: true}, func() *skel.CmdArgs {
				netConf.IPAM.LogLevel = logutils.LogInfoLevelStr
				netConfBytes, err := json.Marshal(netConf)
				Expect(err).NotTo(HaveOccurred())
				args.StdinData = netConfBytes
				return args
			}),
			Entry("release addresses with DEL successfully", ConfigWorkableSets{isPreConfigGood: true, isHealthy: true, isDeleteIPAM: true}, func() *skel.CmdArgs {
				netConf.IPAM.LogLevel = logutils.LogWarnLevelStr
				netConfBytes, err := json.Marshal(netConf)
				Expect(err).NotTo(HaveOccurred())
				args.StdinData = netConfBytes
				return args
			}),
			Entry("failed to release addresses with bad spiderpool agent response", ConfigWorkableSets{isPreConfigGood: true, isHealthy: true, isDeleteIPAM: false}, func() *skel.CmdArgs {
				netConf.IPAM.LogLevel = logutils.LogErrorLevelStr
				netConfBytes, err := json.Marshal(netConf)
				Expect(err).NotTo(HaveOccurred())
				args.StdinData = netConfBytes
				return args
			}),
		)
	})

	Describe("test ipam plugin configuration ", func() {
		It(fmt.Sprintf("[%s] is returning an error on conf broken with ADD/DEL", cniVersion), func() {
			confBytes, err := json.Marshal(netConf)
			Expect(err).NotTo(HaveOccurred())
			confBytes = append(confBytes, []byte("}")...)
			args.StdinData = confBytes

			// Allocate the IP
			go func() {
				defer GinkgoRecover()

				_, _, err := testutils.CmdAddWithArgs(args, func() error {
					return cmd.CmdAdd(args)
				})
				Expect(err).To(HaveOccurred())
				close(addChan)
			}()
			Eventually(addChan).WithTimeout(CNITimeoutSec * time.Second).Should(BeClosed())

			// Release the IP
			go func() {
				defer GinkgoRecover()

				err = testutils.CmdDelWithArgs(args, func() error {
					return cmd.CmdDel(args)
				})
				Expect(err).To(HaveOccurred())
				close(delChan)
			}()
			Eventually(delChan).WithTimeout(CNITimeoutSec * time.Second).Should(BeClosed())
		})

		It(fmt.Sprintf("[%s] is returning an error on bad log configuration with ADD/DEL", cniVersion), func() {
			netConf.IPAM.LogLevel = "bad"
			netConfBytes, err := json.Marshal(netConf)
			Expect(err).NotTo(HaveOccurred())
			args.StdinData = netConfBytes

			// Allocate the IP
			go func() {
				defer GinkgoRecover()

				_, _, err := testutils.CmdAddWithArgs(args, func() error {
					return cmd.CmdAdd(args)
				})
				Expect(err).To(HaveOccurred())
				close(addChan)
			}()
			Eventually(addChan).WithTimeout(CNITimeoutSec * time.Second).Should(BeClosed())

			// Release the IP
			go func() {
				defer GinkgoRecover()

				err = testutils.CmdDelWithArgs(args, func() error {
					return cmd.CmdDel(args)
				})
				Expect(err).To(HaveOccurred())
				close(delChan)
			}()
			Eventually(delChan).WithTimeout(CNITimeoutSec * time.Second).Should(BeClosed())
		})

		It("Check default network configuration", func() {
			// set some configurations with empty value.
			netConf.IPAM.LogLevel = ""
			netConf.IPAM.IPAMUnixSocketPath = ""

			netConfBytes, err := json.Marshal(netConf)
			Expect(err).NotTo(HaveOccurred())

			conf, err := cmd.LoadNetConf(netConfBytes)
			Expect(err).NotTo(HaveOccurred())

			Expect(conf.IPAM.LogLevel).Should(Equal(cmd.DefaultLogLevelStr))
			Expect(conf.IPAM.IPAMUnixSocketPath).Should(Equal(constant.DefaultIPAMUnixSocketPath))
		})

		It("Failed to load args with cmdAdd and cmdDel", func() {
			patches := gomonkey.ApplyFuncSeq(types.LoadArgs, []gomonkey.OutputCell{
				{Values: gomonkey.Params{constant.ErrUnknown}},
				{Values: gomonkey.Params{constant.ErrUnknown}},
			})

			defer patches.Reset()

			confBytes, err := json.Marshal(netConf)
			Expect(err).NotTo(HaveOccurred())
			args.StdinData = confBytes

			// Allocate the IP
			go func() {
				defer GinkgoRecover()

				_, _, err := testutils.CmdAddWithArgs(args, func() error {
					return cmd.CmdAdd(args)
				})
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(constant.ErrUnknown))
				close(addChan)
			}()
			Eventually(addChan).WithTimeout(CNITimeoutSec * time.Second).Should(BeClosed())

			// Release the IP
			go func() {
				defer GinkgoRecover()

				err = testutils.CmdDelWithArgs(args, func() error {
					return cmd.CmdDel(args)
				})
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(constant.ErrUnknown))
				close(delChan)
			}()
			Eventually(delChan).WithTimeout(CNITimeoutSec * time.Second).Should(BeClosed())
		})

		It("Failed to new agent openAPI unix client with cmdAdd and cmdDel", func() {
			patches := gomonkey.ApplyFuncSeq(openapi.NewAgentOpenAPIUnixClient, []gomonkey.OutputCell{
				{Values: gomonkey.Params{nil, constant.ErrUnknown}},
				{Values: gomonkey.Params{nil, constant.ErrUnknown}},
			})
			defer patches.Reset()

			confBytes, err := json.Marshal(netConf)
			Expect(err).NotTo(HaveOccurred())
			args.StdinData = confBytes

			// Allocate the IP
			go func() {
				defer GinkgoRecover()

				_, _, err := testutils.CmdAddWithArgs(args, func() error {
					return cmd.CmdAdd(args)
				})
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(constant.ErrUnknown))
				close(addChan)
			}()
			Eventually(addChan).WithTimeout(CNITimeoutSec * time.Second).Should(BeClosed())

			// Release the IP
			go func() {
				defer GinkgoRecover()

				err = testutils.CmdDelWithArgs(args, func() error {
					return cmd.CmdDel(args)
				})
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(constant.ErrUnknown))
				close(delChan)
			}()
			Eventually(delChan).WithTimeout(CNITimeoutSec * time.Second).Should(BeClosed())
		})
	})

})

func getHealthHandleFunc(isHealthy bool) http.HandlerFunc {
	var healthHandleFunc http.HandlerFunc

	if isHealthy {
		healthHandleFunc = ghttp.RespondWith(connectivity.GetIpamHealthyOKCode, nil)
	} else {
		healthHandleFunc = ghttp.RespondWith(connectivity.GetIpamHealthyInternalServerErrorCode, nil)
	}

	return healthHandleFunc
}
