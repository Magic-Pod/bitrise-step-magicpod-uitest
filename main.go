package main

import (
	"encoding/json"
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
	BaseURL                  string          `env:"base_url,required"`
	APIToken                 stepconf.Secret `env:"magic_pod_api_token,required"`
	OrganizationName         string          `env:"organization_name,required"`
	ProjectName              string          `env:"project_name,required"`
	Environment              string          `env:"environment,required"`
	ExternalServiceToken     stepconf.Secret `env:"external_service_token"`
	ExternalServiceServerURL string          `env:"external_service_server_url"`
	ExternalServiceUserName  string          `env:"external_service_user_name"`
	ExternalServicePassword  stepconf.Secret `env:"external_service_password"`
	OsName                   string          `env:"os,required"`
	DeviceType               string          `env:"device_type,required"`
	Version                  string          `env:"version,required"`
	Model                    string          `env:"model,required"`
	AppType                  string          `env:"app_type,required"`
	AppPath                  string          `env:"app_path"`
	AppURL                   string          `env:"app_url"`
	BundleID                 string          `env:"bundle_id"`
	AppPackage               string          `env:"app_package"`
	AppActivity              string          `env:"app_activity"`
	WaitForResult            bool            `env:"wait_for_result"`
	SendMail                 string          `env:"send_mail"`
	RetryCount               int             `env:"retry_count"`
	CaptureType              string          `env:"capture_type,required"`
	DeviceLanguage           string          `env:"device_language"`
	DeviceRegion             string          `env:"device_region"`
	MultiLangData            string          `env:"multi_lang_data"`
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
	Detail   string                 `json:"detail"`
	ErrorMap map[string]interface{} `json:"."`
}

func failf(format string, v ...interface{}) {
	log.Errorf(format, v...)
	os.Exit(1)
}

func handleError(resp *resty.Response, err error) {
	if err != nil {
                failf(resp.Status())
	}
	if resp.StatusCode() != 200 {
		errorResp := resp.Error().(*ErrorResponse)
		if errorResp.Detail != "" {
			failf("%s: %s", resp.Status(), errorResp.Detail)
		} else {
			var result map[string][]string
			log.Errorf("%s:", resp.Status())
			if err := json.Unmarshal([]byte(resp.String()), &result); err != nil {
				// Unexpectedly returned HTML
				os.Exit(1)
			}
			for key, value := range result {
				log.Errorf("\t%s: %s", key, strings.Join(value, ","))
			}
			os.Exit(1)
		}
	}
}

// Converts parameters for API call but also validates if any of parameters has a `unselectable` value from GUI(e.g. Okinawa dialect for `Device Language`).
// We prefer not to validate parameters because it duplicates the API logic on server  
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
	cfg.DeviceRegion, err = convertDeviceRegionParam(cfg.DeviceRegion)
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
	case "Remote TestKit Onpremise":
		return "remote_testkit_onpremise", nil
	default:
		return "", errors.New("Environment should be 'Magic Pod', 'Remote TestKit' or 'Remote TestKit Onpremise'")
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
	case "Every step":
		return "on_each_step", nil
	case "Every UI transit":
		return "on_ui_transit", nil
	case "Failure capture only":
		return "on_error", nil
	default:
		return "", errors.New("Capture type should be either of 'Every step', 'Every UI transit', or 'Failure capture only'")
	}
}

func convertDeviceLanguageParam(input string) (string, error) {
	switch input {
	case "Default":
		return "default", nil
	case "English":
		return "en", nil
	case "Japanese":
		return "ja", nil
	default:
		return "", errors.New("Device language should be 'Default', 'English' or 'Japanese'")
	}
}

func convertDeviceRegionParam(input string) (string, error) {
	switch input {
	case "Default":
		return "Default", nil
	case "Australia":
		return "AU", nil
	case "Brazil":
		return "BR", nil
	case "Canada":
		return "CA", nil
	case "China mainland":
		return "CN", nil
	case "France":
		return "FR", nil
	case "Germany":
		return "DE", nil
	case "India":
		return "IN", nil
	case "Indonesia":
		return "ID", nil
	case "Italy":
		return "IT", nil
	case "Japan":
		return "JP", nil
	case "Mexico":
		return "MX", nil
	case "Netherlands":
		return "NL", nil
	case "Russia":
		return "RU", nil
	case "Saudi Arabia":
		return "SA", nil
	case "South Korea":
		return "KR", nil
	case "Spain":
		return "ES", nil
	case "Switzerland":
		return "CH", nil
	case "Taiwan":
		return "TW", nil
	case "Turkey":
		return "TR", nil
	case "United Kingdom":
		return "GB", nil
	case "United States":
		return "US", nil
	default:
		return "", errors.New("Invalid Device Region")
	}
}

func createStartBatchRunParams(cfg Config, appFileNumber int) map[string]interface{} {
	params := map[string]interface{}{}

	params["environment"] = cfg.Environment
	if cfg.Environment == "remote_testkit" {
		params["external_service_token"] = cfg.ExternalServiceToken
	} else if cfg.Environment == "remote_testkit_onpremise" {
		params["external_service_server_url"] = cfg.ExternalServiceServerURL
		params["external_service_user_name"] = cfg.ExternalServiceUserName
		params["external_service_password"] = cfg.ExternalServicePassword
	}
	params["os"] = cfg.OsName
	params["device_type"] = cfg.DeviceType
	params["version"] = cfg.Version
	params["model"] = cfg.Model
	params["app_type"] = cfg.AppType
	switch cfg.AppType {
	case "app_file":
		params["app_file_number"] = appFileNumber
		if cfg.OsName == "ios" {
			if cfg.Environment == "remote_testkit" {
				params["bundle_id"] = cfg.BundleID				
			} else if cfg.Environment == "remote_testkit_onpremise" {
				params["bundle_id"] = cfg.BundleID
			}
		}
		break
	case "app_url":
		params["app_url"] = cfg.AppURL
		if cfg.OsName == "ios" {
			if cfg.Environment == "remote_testkit" {
				params["bundle_id"] = cfg.BundleID
			} else if cfg.Environment == "remote_testkit_onpremise" {
				params["bundle_id"] = cfg.BundleID
			}
		}
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
	params["device_region"] = cfg.DeviceRegion
	if cfg.MultiLangData != "" {
		params["shared_data_pattern"] = map[string]string{"multi_lang_data": cfg.MultiLangData}
	}

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
	fmt.Println()
	return zipPath
}

func uploadAppFile(cfg Config) int {
	appPath := cfg.AppPath
	if cfg.OsName == "ios" && cfg.DeviceType == "simulator" {
		appPath = zipAppDir(appPath)
	}
	log.Infof("Upload app file %s to Magic Pod cloud", appPath)

	resp, err := createBaseRequest(cfg).
		SetFile("file", appPath).
		SetResult(UploadFile{}).
		Post("/{organization_name}/{project_name}/upload-file/")
	handleError(resp, err)
	fileNo := resp.Result().(*UploadFile).FileNo
	log.Donef("Done. File number = %d\n", fileNo)
	return fileNo
}

func startBatchRun(cfg Config, appFileNumber int) *BatchRun {
	log.Infof("Start batch run")
	resp, err := createBaseRequest(cfg).
		SetResult(BatchRun{}).
		SetBody(createStartBatchRunParams(cfg, appFileNumber)).
		Post("/{organization_name}/{project_name}/batch-run/")
	handleError(resp, err)
	batchRun := resp.Result().(*BatchRun)
	log.Donef("Batch run #%d has started. You can check detail progress on %s\n",
		batchRun.BatchRunNumber, batchRun.URL)
	return batchRun
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
	if err := os.Unsetenv("external_service_token"); err != nil {
		failf("Failed to remove external service API key data from envs, error: %s", err)
	}
	if err := os.Unsetenv("external_service_password"); err != nil {
		failf("Failed to remove external service password key data from envs, error: %s", err)
	}

	// Upload app file if necessary
	appFileNumber := -1
	if cfg.AppType == "app_file" {
		appFileNumber = uploadAppFile(cfg)
	}

	// Post request to start batch run
	batchRun := startBatchRun(cfg, appFileNumber)
	tools.ExportEnvironmentWithEnvman("MAGIC_POD_TEST_URL", batchRun.URL)

	if !cfg.WaitForResult {
		log.Successf("Exit this step because 'Wait for result' is set to false")
		os.Exit(0)
	}

	// Wait for test finished
	log.Infof("Waiting for the test result ...")
	passedTime := 0
	batchRunNumber := batchRun.BatchRunNumber
	for {
		batchRun = getBatchRun(cfg, batchRunNumber)
		print(".")
		if batchRun.Status != "running" {
			break
		}
		time.Sleep(15 * time.Second)
		passedTime += 15
	}

	// Show result
	testCases := batchRun.TestCases
	message := fmt.Sprintf("\nMagic Pod test %s: \n"+
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
		log.Successf(message)
	default:
		failf(message)
	}

	os.Exit(0)
}
