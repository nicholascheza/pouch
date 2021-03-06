package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/alibaba/pouch/apis/types"
	"github.com/alibaba/pouch/test/command"
	"github.com/alibaba/pouch/test/daemon"
	"github.com/alibaba/pouch/test/environment"
	"github.com/alibaba/pouch/test/util"

	"github.com/go-check/check"
	"github.com/gotestyourself/gotestyourself/icmd"
)

// PouchDaemonSuite is the test suite for daemon.
type PouchDaemonSuite struct{}

func init() {
	check.Suite(&PouchDaemonSuite{})
}

// SetUpTest does common setup in the beginning of each test.
func (suite *PouchDaemonSuite) SetUpTest(c *check.C) {
	SkipIfFalse(c, environment.IsLinux)
}

// TestDaemonCgroupParent tests daemon with cgroup parent
func (suite *PouchDaemonSuite) TestDaemonCgroupParent(c *check.C) {
	dcfg, err := StartDefaultDaemonDebug("--cgroup-parent=tmp")
	if err != nil {
		c.Skip("daemon start failed")
	}

	// Must kill it, as we may loose the pid in next call.
	defer dcfg.KillDaemon()

	cname := "TestDaemonCgroupParent"
	{

		result := RunWithSpecifiedDaemon(dcfg, "pull", busyboxImage)
		if result.ExitCode != 0 {
			dcfg.DumpLog()
			c.Fatalf("pull image failed, err:%v", result)
		}
	}
	{
		result := RunWithSpecifiedDaemon(dcfg, "run",
			"-d", "--name", cname, busyboxImage, "top")
		if result.ExitCode != 0 {
			dcfg.DumpLog()
			c.Fatalf("run container failed, err:%v", result)
		}
	}
	defer RunWithSpecifiedDaemon(dcfg, "rm", "-f", cname)

	// test if the value is in inspect result
	output := RunWithSpecifiedDaemon(dcfg, "inspect", cname).Stdout()
	result := []types.ContainerJSON{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		c.Errorf("failed to decode inspect output: %v", err)
	}

	//// test if cgroup has the right parent path
	//path := fmt.Sprintf("/sys/fs/cgroup/memory/tmp/%s", result.ID)
	//_, err = os.Stat(path)
	//if err != nil {
	//	daemon.DConfig.DumpLog()
	//	c.Fatalf("get cgroup path failed, err:%s", err)
	//}
}

// TestDaemonListenTCP tests daemon listen with tcp address.
func (suite *PouchDaemonSuite) TestDaemonListenTCP(c *check.C) {
	// Start a test daemon with test args.
	listeningPorts := [][]string{
		{"0.0.0.0", "0.0.0.0", "1236"},
		{"127.0.0.1", "127.0.0.1", "1234"},
		{"localhost", "127.0.0.1", "1235"},
	}

	for _, hostDirective := range listeningPorts {
		addr := fmt.Sprintf("tcp://%s:%s", hostDirective[0], hostDirective[2])
		dcfg := daemon.NewConfig()
		dcfg.Listen = ""
		dcfg.NewArgs("--listen=" + addr)
		dcfg.Debug = true
		err := dcfg.StartDaemon()
		c.Assert(err, check.IsNil)

		// verify listen to tcp works
		result := command.PouchRun("--host", addr, "version")
		dcfg.KillDaemon()
		result.Assert(c, icmd.Success)
	}
}

//// TestDaemonConfigFile tests start daemon with configure file works.
//func (suite *PouchDaemonSuite) TestDaemonConfigFile(c *check.C) {
//	path := "/tmp/pouch.json"
//
//	// Unmarshal config.Config, all fields in this struct could be handled in configuration file.
//	cfg := config.Config{
//		Debug: true,
//	}
//	err := CreateConfigFile(path, cfg)
//	c.Assert(err, check.IsNil)
//	defer os.Remove(path)
//
//	dcfg, err := StartDefaultDaemonDebug("--config-file=" + path)
//	{
//		err := dcfg.StartDaemon()
//		c.Assert(err, check.IsNil)
//	}
//
//	// Must kill it, as we may loose the pid in next call.
//	defer dcfg.KillDaemon()
//}

// TestDaemonConfigFileConflict tests start daemon with configure file conflicts with parameter.
func (suite *PouchDaemonSuite) TestDaemonConfigFileConflict(c *check.C) {
	path := "/tmp/pouch.json"
	cfg := struct {
		ContainerdPath string `json:"containerd-path"`
	}{
		ContainerdPath: "abc",
	}
	err := CreateConfigFile(path, cfg)
	c.Assert(err, check.IsNil)
	defer os.Remove(path)

	dcfg, err := StartDefaultDaemon("--containerd-path", "def", "--config-file="+path)
	dcfg.KillDaemon()
	c.Assert(err, check.NotNil)
}

// TestDaemonNestObjectConflict tests start daemon with configure file contains nest objects conflicts with parameter.
func (suite *PouchDaemonSuite) TestDaemonNestObjectConflict(c *check.C) {
	path := "/tmp/pouch_nest.json"
	type TLSConfig struct {
		CA               string `json:"tlscacert,omitempty"`
		Cert             string `json:"tlscert,omitempty"`
		Key              string `json:"tlskey,omitempty"`
		VerifyRemote     bool   `json:"tlsverify"`
		ManagerWhiteList string `json:"manager-whitelist"`
	}
	cfg := struct {
		TLS TLSConfig
	}{
		TLS: TLSConfig{
			CA:   "ca",
			Cert: "cert",
			Key:  "key",
		},
	}
	err := CreateConfigFile(path, cfg)
	c.Assert(err, check.IsNil)
	defer os.Remove(path)

	dcfg, err := StartDefaultDaemon("--tlscacert", "ca", "--config-file="+path)
	dcfg.KillDaemon()
	c.Assert(err, check.NotNil)
}

// TestDaemonSliceFlagNotConflict tests start daemon with configure file contains slice flag will not conflicts with parameter.
func (suite *PouchDaemonSuite) TestDaemonSliceFlagNotConflict(c *check.C) {
	path := "/tmp/pouch_slice.json"
	cfg := struct {
		Labels []string `json:"label"`
	}{
		Labels: []string{"a=a", "b=b"},
	}
	err := CreateConfigFile(path, cfg)
	c.Assert(err, check.IsNil)
	defer os.Remove(path)

	dcfg, err := StartDefaultDaemon("--label", "c=d", "--config-file="+path)
	dcfg.KillDaemon()
	c.Assert(err, check.IsNil)
}

// TestDaemonConfigFileAndCli tests start daemon with configure file and CLI .
func (suite *PouchDaemonSuite) TestDaemonConfigFileAndCli(c *check.C) {
	// Check default configure file could work
	path := "/etc/pouch/config.json"
	cfg := struct {
		Labels []string `json:"label,omitempty"`
	}{
		Labels: []string{"a=b"},
	}
	err := CreateConfigFile(path, cfg)
	c.Assert(err, check.IsNil)
	defer os.Remove(path)

	// Do Not specify configure file explicitly, it should work.
	dcfg, err := StartDefaultDaemonDebug()
	c.Assert(err, check.IsNil)
	defer dcfg.KillDaemon()

	result := RunWithSpecifiedDaemon(dcfg, "info")
	err = util.PartialEqual(result.Stdout(), "a=b")
	c.Assert(err, check.IsNil)
}

// TestDaemonInvalideArgs tests invalid args in daemon return error
func (suite *PouchDaemonSuite) TestDaemonInvalideArgs(c *check.C) {
	_, err := StartDefaultDaemon("--config=xxx")
	c.Assert(err, check.NotNil)
}

// TestDaemonRestart tests daemon restart with running container.
func (suite *PouchDaemonSuite) TestDaemonRestart(c *check.C) {
	dcfg, err := StartDefaultDaemonDebug()
	// Start a test daemon with test args.
	if err != nil {
		c.Skip("daemon start failed.")
	}
	// Must kill it, as we may loose the pid in next call.
	defer dcfg.KillDaemon()

	{
		result := RunWithSpecifiedDaemon(dcfg, "pull", busyboxImage)
		if result.ExitCode != 0 {
			dcfg.DumpLog()
			c.Fatalf("pull image failed, err:%v", result)
		}
	}

	cname := "TestDaemonRestart"
	{
		result := RunWithSpecifiedDaemon(dcfg, "run", "-d", "--name", cname,
			"-p", "1234:80",
			busyboxImage, "top")
		if result.ExitCode != 0 {
			dcfg.DumpLog()
			c.Fatalf("run container failed, err:%v", result)
		}
	}
	defer RunWithSpecifiedDaemon(dcfg, "rm", "-f", cname)

	// restart daemon
	err = RestartDaemon(dcfg)
	c.Assert(err, check.IsNil)

	// test if the container is running.
	output := RunWithSpecifiedDaemon(dcfg, "inspect", cname).Stdout()
	result := []types.ContainerJSON{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		c.Fatalf("failed to decode inspect output: %v", err)
	}
	c.Assert(string(result[0].State.Status), check.Equals, "running")
}

// TestDaemonRestartWithPausedContainer tests daemon with paused container.
func (suite *PouchDaemonSuite) TestDaemonRestartWithPausedContainer(c *check.C) {
	dcfg, err := StartDefaultDaemonDebug()
	//Start a test daemon with test args.
	if err != nil {
		c.Skip("daemon start failed")
	}
	defer dcfg.KillDaemon()

	{
		result := RunWithSpecifiedDaemon(dcfg, "pull", busyboxImage)
		if result.ExitCode != 0 {
			dcfg.DumpLog()
			c.Fatalf("pull image failed, err: %v", result)
		}
	}

	cname := "TestDaemonRestartWithPausedContainer"
	{
		result := RunWithSpecifiedDaemon(dcfg, "run", "-d", "--name", cname,
			"-p", "5678:80", busyboxImage, "top")
		if result.ExitCode != 0 {
			dcfg.DumpLog()
			c.Fatalf("run container failed, err: %v", result)
		}

		// pause the container
		result = RunWithSpecifiedDaemon(dcfg, "pause", cname)
		if result.ExitCode != 0 {
			dcfg.DumpLog()
			c.Fatalf("pause container failed, err: %v", result)
		}
	}
	defer RunWithSpecifiedDaemon(dcfg, "rm", "-f", cname)

	// restart daemon
	err = RestartDaemon(dcfg)
	c.Assert(err, check.IsNil)

	// test if the container is paused.
	output := RunWithSpecifiedDaemon(dcfg, "inspect", cname).Stdout()
	data := []types.ContainerJSON{}
	if err := json.Unmarshal([]byte(output), &data); err != nil {
		c.Fatalf("failed to decode inspect output: %v", err)
	}
	c.Assert(string(data[0].State.Status), check.Equals, "paused")

	// unpause the container
	result := RunWithSpecifiedDaemon(dcfg, "unpause", cname)
	if result.ExitCode != 0 {
		dcfg.DumpLog()
		c.Fatalf("unpause container failed, err: %v", result)
	}

	//test if the container is running
	output = RunWithSpecifiedDaemon(dcfg, "inspect", cname).Stdout()
	if err := json.Unmarshal([]byte(output), &data); err != nil {
		c.Fatalf("failed to decode inspect output: %v", err)
	}
	c.Assert(string(data[0].State.Status), check.Equals, "running")
}

// TestDaemonLabel tests start daemon with label works.
func (suite *PouchDaemonSuite) TestDaemonLabel(c *check.C) {
	dcfg, err := StartDefaultDaemonDebug("--label", "a=b")
	// Start a test daemon with test args.
	if err != nil {
		c.Skip("daemon start failed.")
	}
	// Must kill it, as we may loose the pid in next call.
	defer dcfg.KillDaemon()

	result := RunWithSpecifiedDaemon(dcfg, "info")
	err = util.PartialEqual(result.Stdout(), "a=b")
	c.Assert(err, check.IsNil)
}

// TestDaemonLabelDup tests start daemon with duplicated label works.
func (suite *PouchDaemonSuite) TestDaemonLabelDup(c *check.C) {
	dcfg, err := StartDefaultDaemonDebug("--label", "a=b", "--label", "a=b")
	// Start a test daemon with test args.
	if err != nil {
		c.Skip("daemon start failed.")
	}
	// Must kill it, as we may loose the pid in next call.
	defer dcfg.KillDaemon()

	result := RunWithSpecifiedDaemon(dcfg, "info")
	err = util.PartialEqual(result.Stdout(), "a=b")
	c.Assert(err, check.IsNil)

	cnt := strings.Count(result.Stdout(), "a=b")
	c.Assert(cnt, check.Equals, 1)
}

// TestDaemonLabelNeg tests start daemon with wrong label could not work.
func (suite *PouchDaemonSuite) TestDaemonLabelNeg(c *check.C) {
	_, err := StartDefaultDaemon("--label", "adsf")
	c.Assert(err, check.NotNil)
}

// TestDaemonDefaultRegistry tests set default registry works.
func (suite *PouchDaemonSuite) TestDaemonDefaultRegistry(c *check.C) {
	dcfg, err := StartDefaultDaemonDebug(
		"--default-registry",
		"reg.docker.alibaba-inc.com",
		"--default-registry-namespace",
		"base")
	c.Assert(err, check.IsNil)

	// Check pull image with default registry using the registry specified in daemon.
	result := RunWithSpecifiedDaemon(dcfg, "pull", "hello-world")
	err = util.PartialEqual(result.Combined(), "reg.docker.alibaba-inc.com/base/hello-world")
	c.Assert(err, check.IsNil)

	defer dcfg.KillDaemon()
}

// TestDaemonCriEnabled tests enabling cri part in pouchd.
func (suite *PouchDaemonSuite) TestDaemonCriEnabled(c *check.C) {
	dcfg, err := StartDefaultDaemonDebug(
		"--enable-cri")
	c.Assert(err, check.IsNil)

	result := RunWithSpecifiedDaemon(dcfg, "info")
	err = util.PartialEqual(result.Combined(), "CriEnabled: true")
	c.Assert(err, check.IsNil)

	defer dcfg.KillDaemon()
}

// TestDaemonTlsVerify tests start daemon with TLS verification enabled.
func (suite *PouchDaemonSuite) TestDaemonTlsVerify(c *check.C) {
	SkipIfFalse(c, IsTLSExist)
	dcfg := daemon.NewConfig()
	dcfg.Listen = ""
	dcfg.NewArgs("--listen=" + testDaemonHTTPSAddr)
	dcfg.Args = append(dcfg.Args,
		"--tlsverify",
		"--tlscacert="+serverCa,
		"--tlscert="+serverCert,
		"--tlskey="+serverKey)
	dcfg.Debug = false
	// Skip error check, because the function to check daemon up using CLI without TLS info.
	dcfg.StartDaemon()

	// Must kill it, as we may loose the pid in next call.
	defer dcfg.KillDaemon()

	// Use TLS could success
	result := RunWithSpecifiedDaemon(&dcfg,
		"--tlscacert="+clientCa,
		"--tlscert="+clientCert,
		"--tlskey="+clientKey, "version")
	result.Assert(c, icmd.Success)

	// Do not use TLS should fail
	result = RunWithSpecifiedDaemon(&dcfg, "version")
	c.Assert(result.ExitCode, check.Equals, 1)
	err := util.PartialEqual(result.Stderr(), "malformed HTTP response")
	c.Assert(err, check.IsNil)

	{
		// Use wrong CA should fail
		result := RunWithSpecifiedDaemon(&dcfg,
			"--tlscacert="+clientWrongCa,
			"--tlscert="+clientCert,
			"--tlskey="+clientKey, "version")
		c.Assert(result.ExitCode, check.Equals, 1)
		err := util.PartialEqual(result.Stderr(), "failed to append certificates")
		c.Assert(err, check.IsNil)
	}
}

// TestDaemonStartOverOneTimes tests start daemon over one times should fail.
func (suite *PouchDaemonSuite) TestDaemonStartOverOneTimes(c *check.C) {
	dcfg1 := daemon.NewConfig()
	dcfg1.Listen = ""
	addr1 := "unix:///var/run/pouchtest1.sock"
	dcfg1.NewArgs("--listen=" + addr1)
	err := dcfg1.StartDaemon()
	c.Assert(err, check.IsNil)

	// verify listen to tcp works
	command.PouchRun("--host", addr1, "version").Assert(c, icmd.Success)
	defer dcfg1.KillDaemon()

	// test second daemon with same pidfile should start fail
	dcfg2 := daemon.NewConfig()
	dcfg2.Listen = ""
	addr2 := "unix:///var/run/pouchtest2.sock"
	dcfg2.NewArgs("--listen=" + addr2)
	err = dcfg2.StartDaemon()
	c.Assert(err, check.NotNil)

}

// TestDaemonWithMultiRuntimes tests start daemon with multiple runtimes
func (suite *PouchDaemonSuite) TestDaemonWithMultiRuntimes(c *check.C) {
	dcfg1, err := StartDefaultDaemonDebug(
		"--add-runtime", "foo=bar")
	c.Assert(err, check.IsNil)
	dcfg1.KillDaemon()

	// should fail if runtime name equal
	dcfg2, err := StartDefaultDaemonDebug(
		"--add-runtime", "runa=runa",
		"--add-runtime", "runa=runa")
	c.Assert(err, check.NotNil)
	dcfg2.KillDaemon()
}

// TestRestartStoppedContainerAfterDaemonRestart is used to test the case that
// when container is stopped and then pouchd restarts, the restore logic should
// initialize the existing container IO settings even though they are not alive.
func (suite *PouchDaemonSuite) TestRestartStoppedContainerAfterDaemonRestart(c *check.C) {
	c.Skip("The wait command can't guarantee container cleanup job can be done before api return")

	cfgFile := filepath.Join("/tmp", c.TestName())
	c.Assert(CreateConfigFile(cfgFile, nil), check.IsNil)
	defer os.RemoveAll(cfgFile)

	cfg := daemon.NewConfig()
	cfg.NewArgs("--config-file", cfgFile)
	c.Assert(cfg.StartDaemon(), check.IsNil)

	defer cfg.KillDaemon()

	var (
		cname = c.TestName()
		msg   = "hello"
	)

	// pull image
	RunWithSpecifiedDaemon(&cfg, "pull", busyboxImage).Assert(c, icmd.Success)

	// run a container
	res := RunWithSpecifiedDaemon(&cfg, "run", "--name", cname, busyboxImage, "echo", msg)
	defer ensureContainerNotExist(&cfg, cname)

	res.Assert(c, icmd.Success)
	c.Assert(strings.TrimSpace(res.Combined()), check.Equals, msg)

	// wait for it.
	RunWithSpecifiedDaemon(&cfg, "wait", cname).Assert(c, icmd.Success)

	// kill the daemon and make sure it has been killed
	cfg.KillDaemon()
	c.Assert(cfg.IsDaemonUp(), check.Equals, false)

	// restart again
	c.Assert(cfg.StartDaemon(), check.IsNil)

	// start the container again
	res = RunWithSpecifiedDaemon(&cfg, "start", "-a", cname)
	res.Assert(c, icmd.Success)
	c.Assert(strings.TrimSpace(res.Combined()), check.Equals, msg)
}

// TestUpdateDaemonWithLabels tests update daemon online with labels updated
func (suite *PouchDaemonSuite) TestUpdateDaemonWithLabels(c *check.C) {
	cfg := daemon.NewConfig()
	err := cfg.StartDaemon()
	c.Assert(err, check.IsNil)

	defer cfg.KillDaemon()

	RunWithSpecifiedDaemon(&cfg, "updatedaemon", "--label", "aaa=bbb").Assert(c, icmd.Success)

	ret := RunWithSpecifiedDaemon(&cfg, "info")
	ret.Assert(c, icmd.Success)

	updated := strings.Contains(ret.Stdout(), "aaa=bbb")
	c.Assert(updated, check.Equals, true)
}

// TestUpdateDaemonWithLabels tests update daemon offline
func (suite *PouchDaemonSuite) TestUpdateDaemonOffline(c *check.C) {
	path := "/tmp/pouchconfig.json"
	fd, err := os.Create(path)
	c.Assert(err, check.IsNil)
	fd.Close()
	defer os.Remove(path)

	cfg := daemon.NewConfig()
	err = cfg.StartDaemon()
	c.Assert(err, check.IsNil)

	defer cfg.KillDaemon()

	RunWithSpecifiedDaemon(&cfg, "updatedaemon", "--config-file", path, "--offline=true").Assert(c, icmd.Success)

	ret := RunWithSpecifiedDaemon(&cfg, "info")
	ret.Assert(c, icmd.Success)
}

func ensureContainerNotExist(dcfg *daemon.Config, cname string) error {
	_ = RunWithSpecifiedDaemon(dcfg, "rm", "-f", cname)
	return nil
}

// TestRecoverContainerWhenHostDown tests when the host down, the pouchd still can
// recover the container whose restart policy is always .
func (suite *PouchDaemonSuite) TestRecoverContainerWhenHostDown(c *check.C) {
	dcfg, err := StartDefaultDaemonDebug()
	//Start a test daemon with test args.
	if err != nil {
		c.Skip("daemon start failed")
	}
	defer dcfg.KillDaemon()

	cname := "TestRecoverContainerWhenHostDown"
	ensureContainerNotExist(dcfg, cname)

	// prepare test image
	result := RunWithSpecifiedDaemon(dcfg, "pull", busyboxImage)
	if result.ExitCode != 0 {
		dcfg.DumpLog()
		c.Fatalf("pull image failed, err: %v", result)
	}

	result = RunWithSpecifiedDaemon(dcfg, "run", "-d", "--name", cname, "--restart", "always", busyboxImage, "top")
	if result.ExitCode != 0 {
		dcfg.DumpLog()
		c.Fatalf("run container failed, err: %v", result)
	}
	defer ensureContainerNotExist(dcfg, cname)

	// get the container init process id
	pidStr := RunWithSpecifiedDaemon(dcfg, "inspect", "-f", "{{.State.Pid}}", cname).Stdout()

	// get parent pid of container init process
	output, err := exec.Command("ps", "-o", "ppid=", "-p", strings.TrimSpace(pidStr)).Output()
	if err != nil {
		c.Errorf("failed to get parent pid of container %s: output: %s err: %v", cname, string(output), err)
	}
	// imitate the host down
	// first kill the daemon
	dcfg.KillDaemon()

	// second kill the container's process
	ppid, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		dcfg.DumpLog()
		c.Fatalf("failed to convert pid string %s to int: %v", output, err)
	}
	syscall.Kill(ppid, syscall.SIGKILL)

	// restart the daemon
	err = RestartDaemon(dcfg)
	c.Assert(err, check.IsNil)

	// wait container started again or timeout error
	check := make(chan struct{})
	timeout := make(chan bool, 1)
	// set timeout to wait container started
	go func() {
		time.Sleep(10 * time.Second)
		timeout <- true
	}()

	// check whether container started
	go func() {
		for {
			data := RunWithSpecifiedDaemon(dcfg, "inspect", cname).Stdout()
			cInfo := []types.ContainerJSON{}
			if err := json.Unmarshal([]byte(data), &cInfo); err != nil {
				c.Fatalf("failed to decode inspect output: %v", err)
			}

			if len(cInfo) == 0 || cInfo[0].State == nil {
				continue
			}

			if string(cInfo[0].State.Status) == "running" {
				check <- struct{}{}
				break
			}

			fmt.Printf("container %s status: %s\n", cInfo[0].ID, string(cInfo[0].State.Status))
			time.Sleep(1 * time.Second)
		}
	}()

	select {
	case <-check:
	case <-timeout:
		dcfg.DumpLog()
		c.Fatalf("failed to wait container running")
	}
}

// TestDaemonWithSysyemdCgroupDriver tests start daemon with systemd cgroup driver
func (suite *PouchDaemonSuite) TestDaemonWithSystemdCgroupDriver(c *check.C) {
	SkipIfFalse(c, environment.SupportSystemdCgroupDriver)
	tmpDir, err := ioutil.TempDir("", "cgroup-driver")
	path := filepath.Join(tmpDir, "config.json")
	c.Assert(err, check.IsNil)
	cfg := struct {
		CgroupDriver string `json:"cgroup-driver,omitempty"`
	}{
		CgroupDriver: "systemd",
	}
	c.Assert(CreateConfigFile(path, cfg), check.IsNil)
	defer os.RemoveAll(tmpDir)

	dcfg, err := StartDefaultDaemon("--config-file=" + path)
	defer dcfg.KillDaemon()
	c.Assert(err, check.IsNil)

	result := RunWithSpecifiedDaemon(dcfg, "info")
	c.Assert(util.PartialEqual(result.Stdout(), "systemd"), check.IsNil)

	cname := "TestWithSystemdCgroupDriver"
	ret := RunWithSpecifiedDaemon(dcfg, "run", "-d", "--name", cname, busyboxImage, "top")
	defer RunWithSpecifiedDaemon(dcfg, "rm", "-f", cname)
	ret.Assert(c, icmd.Success)
}

// TestContainerdPIDReuse tests even though old containerd pid being reused, we can still
// pull up the containerd instance.
func (suite *PouchDaemonSuite) TestContainerdPIDReuse(c *check.C) {
	cfgFile := filepath.Join("/tmp", c.TestName())
	c.Assert(CreateConfigFile(cfgFile, nil), check.IsNil)
	defer os.RemoveAll(cfgFile)

	// prepare config file for pouchd
	cfg := daemon.NewConfig()
	cfg.NewArgs("--config-file", cfgFile)

	containerdPidPath := filepath.Join("/tmp/test/pouch/containerd/state", "containerd.pid")

	// set containerd pid to 1 to make sure the pid must be alive
	err := ioutil.WriteFile(containerdPidPath, []byte(fmt.Sprintf("%d", 1)), 0660)
	if err != nil {
		c.Errorf("failed to write pid to file: %v", containerdPidPath)
	}

	// make sure pouchd can successfully start
	c.Assert(cfg.StartDaemon(), check.IsNil)
	defer cfg.KillDaemon()
}
