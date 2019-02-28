package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-tools/go-steputils/stepconf"
	"github.com/bitrise-tools/go-steputils/tools"
	"github.com/mholt/archiver"
	"gopkg.in/resty.v1"
)

// Config : Configuration for this step
type Config struct {
	BaseURL              string          `env:"base_url,required"`
	APIToken             stepconf.Secret `env:"magic_pod_api_token,required"`
	OrganizationName     string          `env:"organization_name,required"`
	ProjectName          string          `env:"project_name,required"`
	Environment          string          `env:"environment,required"`
	ExternalServiceToken stepconf.Secret `env:"external_service_token"`
	OsName               string          `env:"os,required"`
	DeviceType           string          `env:"device_type,required"`
	Version              string          `env:"version,required"`
	Model                string          `env:"model,required"`
	AppType              string          `env:"app_type,required"`
	AppPath              string          `env:"app_path"`
	AppURL               string          `env:"app_url"`
	BundleID             string          `env:"bundle_id"`
	AppPackage           string          `env:"app_package"`
	AppActivity          string          `env:"app_activity"`
	SendMail             string          `env:"send_mail"`
	RetryCount           int             `env:"retry_count"`
	CaptureType          string          `env:"capture_type,required"`
	DeviceLanguage       string          `env:"device_language"`
	MultiLangData        string          `env:"multi_lang_data"`
	MaxWaitTime          int             `env:"max_wait_time"`
}

// UploadFile : Response from upload-file API
type UploadFile struct {
	FileName string `json:"file_name"`
	FileNo   int    `json:"file_no"`
}

// TestCases : Part of response from batch-run API. It stands for number of test cases
type TestCases struct {
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
	Total     int `json:"total"`
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
	Detail string `json:"detail"`
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
		failf("%s: %s", resp.Status(), errorResp.Detail)
	}
}

func (cfg *Config) convertToAPIParams() []error {
	var err error
	errors := []error{}
	cfg.Environment, err = convertEnvironmentParam(cfg.Environment)
	if err != nil {
		errors = append(errors, err)
	}
	cfg.OsName = convertToSnakeCase(cfg.OsName)
	cfg.DeviceType = convertToSnakeCase(cfg.DeviceType)
	cfg.AppType, err = convertAppTypeParam(cfg.AppType)
	if err != nil {
		errors = append(errors, err)
	}
	cfg.CaptureType, err = convertCaptureTypeParam(cfg.CaptureType)
	if err != nil {
		errors = append(errors, err)
	}
	cfg.DeviceLanguage, err = convertDeviceLanguageParam(cfg.DeviceLanguage)
	if err != nil {
		errors = append(errors, err)
	}
	return errors
}

func convertToSnakeCase(input string) string {
	converted := strings.ToLower(input)
	converted = strings.Replace(converted, " ", "_", -1)

	return converted
}

func convertEnvironmentParam(input string) (string, error) {
	switch input {
	case "Magic Pod":
		return "magic_pod", nil
	case "Remote TestKit":
		return "remote_testkit", nil
	default:
		return "", errors.New("Environment should be 'Magic Pod' or 'Remote TestKit'")
	}
}

func convertAppTypeParam(input string) (string, error) {
	switch input {
	case "App file (cloud upload)":
		return "app_file", nil
	case "App file (URL)":
		return "app_url", nil
	case "Installed app":
		return "installed", nil
	default:
		return "", errors.New("App type should be either of 'App file (cloud upload)', 'App file (URL)', or 'Installed app'")
	}
}

func convertCaptureTypeParam(input string) (string, error) {
	switch input {
	case "Failure capture only":
		return "on_error", nil
	case "Every UI transit":
		return "on_ui_transit", nil
	case "Every step":
		return "on_each_step", nil
	default:
		return "", errors.New("Capture type should be either of 'Failure capture only', 'Every UI transit', or 'Every step'")
	}
}

func convertDeviceLanguageParam(input string) (string, error) {
	switch input {
	case "English":
		return "en", nil
	case "Japanese":
		return "ja", nil
	default:
		return "", errors.New("Device language should be 'English' or 'Japanese'")
	}
}

func createStartBatchRunParams(cfg Config, appFileNumber int) map[string]interface{} {
	params := map[string]interface{}{}

	params["environment"] = cfg.Environment
	if cfg.Environment != "magic_pod" {
		params["external_service_token"] = cfg.ExternalServiceToken
	}
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
	params["send_mail"] = cfg.SendMail
	params["retry_count"] = cfg.RetryCount
	params["capture_type"] = cfg.CaptureType
	params["device_language"] = cfg.DeviceLanguage
	params["shared_data_pattern_rows"] = map[string]string{"multi_lang_data": cfg.MultiLangData}

	return params
}

func createBaseRequest(cfg Config) *resty.Request {
	return resty.
		SetHostURL(cfg.BaseURL).R().
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
	if err := os.RemoveAll(zipPath); err != nil {
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
		Get("/{organization_name}/{project_name}/batch-run/{batch_run_number}/")
	handleError(resp, err)
	return resp.Result().(*BatchRun)
}

func main() {

	// Parse configuration
	var cfg Config
	if err := stepconf.Parse(&cfg); err != nil {
		failf(err.Error())
	}
	enumErr := cfg.convertToAPIParams()
	if len(enumErr) != 0 {
		for i := range enumErr {
			log.Errorf("- %s", enumErr[i].Error())
		}
		os.Exit(1)
	}

	stepconf.Print(cfg)
	fmt.Println()

	if err := os.Unsetenv("magic_pod_api_token"); err != nil {
		failf("Failed to remove API key data from envs, error: %s", err)
	}
	if err := os.Unsetenv("external_cloud_token"); err != nil {
		failf("Failed to remove external service API key data from envs, error: %s", err)
	}

	// Upload app file if necessary
	appFileNumber := -1
	if cfg.AppType == "app_file" {
		appFileNumber = uploadAppFile(cfg)
	}

	// Post request to start batch run
	batchRunNumber := startBatchRun(cfg, appFileNumber)

	// Wait for test finished
	// var batchRun = BatchRun{}
	batchRun := &BatchRun{}
	log.Infof("Waiting for the test result ...")
	passedTime := 0
	for {
		batchRun = getBatchRun(cfg, batchRunNumber)
		print(".")
		if passedTime >= 60*cfg.MaxWaitTime {
			println()
			log.Errorf("Max waiting time has passed.  This step is marked as failure")
			break
		} else if batchRun.Status != "running" {
			break
		}
		time.Sleep(15 * time.Second)
		passedTime += 15
	}

	// Show result
	testCases := batchRun.TestCases
	message := fmt.Sprintf("Magic Pod test %s: \n"+
		"\tSucceeded : %d\n"+
		"\tFailed : %d\n"+
		"\tTotal : %d\n"+
		"Please see %s for detail",
		batchRun.Status, testCases.Succeeded, testCases.Failed, testCases.Total, batchRun.URL)
	tools.ExportEnvironmentWithEnvman("MAGIC_POD_TEST_STATUS", batchRun.Status)
	tools.ExportEnvironmentWithEnvman("MAGIC_POD_TEST_SUCCEEDED_COUNT", strconv.Itoa(testCases.Succeeded))
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
