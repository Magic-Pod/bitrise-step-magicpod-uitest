package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-tools/go-steputils/stepconf"
	"github.com/bitrise-tools/go-steputils/tools"
	"github.com/mholt/archiver"
	"gopkg.in/resty.v1"
)

// HostURL : Base URL of Magic Pod API
const HostURL = "http://localhost:5000"

// Config : Configuration for this step
type Config struct {
	APIToken         stepconf.Secret `env:"magic_pod_api_token,required"`
	OrganizationName string          `env:"organization_name,required"`
	ProjectName      string          `env:"project_name,required"`
	Environment      string          `env:"environment,required"`
	OsName           string          `env:"os,required"`
	DeviceType       string          `env:"device_type,required"`
	Version          string          `env:"version,required"`
	Model            string          `env:"model,required"`
	AppType          string          `env:"app_type,required"`
	AppPath          string          `env:"app_path"`
	AppURL           string          `env:"app_url"`
	BundleID         string          `env:"bundle_id"`
	AppPackage       string          `env:"app_package"`
	AppActivity      string          `env:"app_activity"`
	CaptureType      string          `env:"capture_type,required"`
	DeviceLanguage   string          `env:"device_language"`
	MultiLangData    string          `env:"multi_lang_data"`
}

// UploadFile : Response from upload-file API
type UploadFile struct {
	FileName string `json:"file_name"`
	FileNo   int    `json:"file_no"`
}

// TestCases : Part of response from batch-run API. It stands for number of test cases
type TestCases struct {
	Passed int `json:"passed"`
	Failed int `json:"failed"`
	Total  int `json:"total"`
}

// BatchRun : Response from batch-run API
type BatchRun struct {
	Organizationname string    `json:"organization_name"`
	ProjectName      string    `json:"project_name"`
	BatchRunNumber   int       `json:"batch_run_number"`
	Status           string    `json:"status"`
	TestCases        TestCases `json:"test_cases"`
	URL              string    `json:"url"`
}

// ErrorResponse : Response from APIs when they are not finished with status 200
type ErrorResponse struct {
	Status int    `json:"status"`
	Detail string `json:"detail"`
	Title  string `json:"title"`
}

func failf(format string, v ...interface{}) {
	log.Errorf(format, v...)
	os.Exit(1)
}

func handleError(resp *resty.Response, err error) {
	if err != nil {
		failf(err.Error())
	}
	if resp.StatusCode() != 200 {
		errorResp := resp.Error().(*ErrorResponse)
		failf("%s: %s", errorResp.Title, errorResp.Detail)
	}
}

func (cfg *Config) convertToAPIParams() {
	cfg.Environment = convertEnvironmentParam(cfg.Environment)
	cfg.OsName = convertToSnakeCase(cfg.OsName)
	cfg.DeviceType = convertToSnakeCase(cfg.DeviceType)
	cfg.AppType = convertAppTypeParam(cfg.AppType)
	cfg.CaptureType = convertCaptureTypeParam(cfg.CaptureType)
}

func convertToSnakeCase(input string) string {
	converted := strings.ToLower(input)
	converted = strings.Replace(converted, " ", "_", -1)

	return converted
}

func convertEnvironmentParam(input string) string {
	switch input {
	case "Magic Pod":
		return "magic_pod"
	case "Remote TestKit":
		return "remote_testkit"
	default:
		failf("Failed to convert Environment %s", input)
		// cannot reach here
		panic("Failed to convert Environment")
	}
}

func convertAppTypeParam(input string) string {
	switch input {
	case "App file (cloud upload)":
		return "app_file"
	case "App file (URL)":
		return "app_url"
	case "Installed app":
		return "installed"
	default:
		failf("Failed to convert App type %s", input)
		// cannot reach here
		panic("Failed to convert App type")
	}
}

func convertCaptureTypeParam(input string) string {
	switch input {
	case "Failure capture only":
		return "on_error"
	case "Every UI transit":
		return "on_ui_transit"
	case "Every step":
		return "on_each_step"
	default:
		failf("Failed to convert Capture type %s", input)
		// cannot reach here
		panic("Failed to convert Capture type")
	}
}

func createStartBatchRunParams(cfg Config, appFileNumber int) map[string]interface{} {
	params := map[string]interface{}{}

	params["environment"] = cfg.Environment
	params["os"] = cfg.OsName
	params["device_type"] = cfg.DeviceType
	params["version"] = cfg.Version
	params["model"] = cfg.Model
	params["app_type"] = cfg.AppType
	switch cfg.AppType {
	case "app_file":
		params["app_file_number"] = appFileNumber
		break
	case "app_url":
		params["app_url"] = cfg.AppURL
		break
	case "installed":
		if cfg.OsName == "ios" {
			params["bundle_id"] = cfg.BundleID
		} else {
			params["app_package"] = cfg.AppPackage
			params["app_activity"] = cfg.AppActivity
		}
		break
	}
	params["capture_type"] = cfg.CaptureType
	params["device_language"] = cfg.DeviceLanguage
	params["shared_data_pattern_rows"] = map[string]string{"multi_lang_data": cfg.MultiLangData}

	return params
}

func createBaseRequest(cfg Config) *resty.Request {
	return resty.
		SetHostURL(HostURL).R().
		SetHeader("Authorization", "Token "+string(cfg.APIToken)).
		SetPathParams(map[string]string{
			"organization_name": cfg.OrganizationName,
			"project_name":      cfg.ProjectName,
		}).
		SetError(ErrorResponse{})
}

func zipAppDir(dirPath string) string {
	log.Infof("Zip app directory %s", dirPath)
	zipPath := dirPath + ".zip"
	if err := os.Remove(zipPath); err != nil {
		failf(err.Error())
	}
	if err := archiver.Archive([]string{dirPath}, zipPath); err != nil {
		failf(err.Error())
	}
	return zipPath
}

func uploadAppFile(cfg Config) int {
	appPath := cfg.AppPath
	if cfg.OsName == "ios" && cfg.DeviceType == "simulator" {
		appPath = zipAppDir(appPath)
	}

	resp, err := createBaseRequest(cfg).
		SetFile("file", appPath).
		SetResult(UploadFile{}).
		Post("/{organization_name}/{project_name}/upload-file/")
	handleError(resp, err)
	return resp.Result().(*UploadFile).FileNo
}

func startBatchRun(cfg Config, appFileNumber int) int {
	resp, err := createBaseRequest(cfg).
		SetResult(BatchRun{}).
		SetBody(createStartBatchRunParams(cfg, appFileNumber)).
		Post("/{organization_name}/{project_name}/batch-run/")
	handleError(resp, err)
	return resp.Result().(*BatchRun).BatchRunNumber
}

func getBatchRun(cfg Config, batchRunNumber int) *BatchRun {
	resp, err := createBaseRequest(cfg).
		SetPathParams(map[string]string{
			"batch_run_number": strconv.Itoa(batchRunNumber),
		}).
		SetResult(BatchRun{}).
		Get("/{organization_name}/{project_name}/batch-run/{batch_run_number}")
	handleError(resp, err)
	return resp.Result().(*BatchRun)
}

func main() {

	// Display configuration
	var cfg Config
	if err := stepconf.Parse(&cfg); err != nil {
		failf("Issue with input: %s", err)
	}
	// TODO error handling
	cfg.convertToAPIParams()

	stepconf.Print(cfg)
	fmt.Println()

	if err := os.Unsetenv("magic_pod_api_token"); err != nil {
		failf("Failed to remove API key data from envs, error: %s", err)
	}

	// Upload app file if necessary
	appFileNumber := -1
	if cfg.AppType == "app_file" {
		appFileNumber = uploadAppFile(cfg)
	}

	// Post request to start batch run
	batchRunNumber := startBatchRun(cfg, appFileNumber)
	log.Infof("batch run number = %d", batchRunNumber)

	// Wait for test finished
	// var batchRun = BatchRun{}
	batchRun := &BatchRun{}
	for {
		batchRun = getBatchRun(cfg, batchRunNumber)
		// TODO avoid infinite loop
		if batchRun.Status != "running" {
			break
		}
	}

	// Show result
	testCases := batchRun.TestCases
	message := fmt.Sprintf("Magic Pod test %s: \n"+
		"\tPassed : %d\n"+
		"\tFailed : %d\n"+
		"\tTotal : %d\n"+
		"Please see %s for detail",
		batchRun.Status, testCases.Passed, testCases.Failed, testCases.Total, batchRun.URL)
	tools.ExportEnvironmentWithEnvman("MAGIC_POD_TEST_STATUS", batchRun.Status)
	tools.ExportEnvironmentWithEnvman("MAGIC_POD_TEST_PASSED_COUNT", strconv.Itoa(testCases.Passed))
	tools.ExportEnvironmentWithEnvman("MAGIC_POD_TEST_FAILED_COUNT", strconv.Itoa(testCases.Failed))
	tools.ExportEnvironmentWithEnvman("MAGIC_POD_TEST_TOTAL_COUNT", strconv.Itoa(testCases.Total))
	switch batchRun.Status {
	case "succeeded":
		log.Infof(message)
	default:
		failf(message)
	}

	log.Donef("Test succeeded")
	os.Exit(0)
}
