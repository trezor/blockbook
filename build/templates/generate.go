package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/template"
	"time"
)

const (
	inputDir  = "templates"
	outputDir = "pkg-defs"
)

type Config struct {
	Meta struct {
		BuildDatetime          string // generated field
		PackageMaintainer      string `json:"package_maintainer"`
		PackageMaintainerEmail string `json:"package_maintainer_email"`
	}
	Env struct {
		Version              string `json:"version"`
		BackendInstallPath   string `json:"backend_install_path"`
		BackendDataPath      string `json:"backend_data_path"`
		BlockbookInstallPath string `json:"blockbook_install_path"`
		BlockbookDataPath    string `json:"blockbook_data_path"`
	} `json:"env"`
	Coin struct {
		Name     string `json:"name"`
		Shortcut string `json:"shortcut"`
		Label    string `json:"label"`
		Alias    string `json:"alias"`
	} `json:"coin"`
	Ports struct {
		BackendRPC          int `json:"backend_rpc"`
		BackendMessageQueue int `json:"backend_message_queue"`
		BlockbookInternal   int `json:"blockbook_internal"`
		BlockbookPublic     int `json:"blockbook_public"`
	} `json:"ports"`
	BlockChain struct {
		RPCURLTemplate              string                     `json:"rpc_url_template"`
		RPCUser                     string                     `json:"rpc_user"`
		RPCPass                     string                     `json:"rpc_pass"`
		RPCTimeout                  int                        `json:"rpc_timeout"`
		Parse                       bool                       `json:"parse"`
		MessageQueueBindingTemplate string                     `json:"message_queue_binding_template"`
		Subversion                  string                     `json:"subversion"`
		AddressFormat               string                     `json:"address_format"`
		MempoolWorkers              int                        `json:"mempool_workers"`
		MempoolSubWorkers           int                        `json:"mempool_sub_workers"`
		BlockAddressesToKeep        int                        `json:"block_addresses_to_keep"`
		AdditionalParams            map[string]json.RawMessage `json:"additional_params"`
	} `json:"block_chain"`
	Backend struct {
		PackageName            string      `json:"package_name"`
		PackageRevision        string      `json:"package_revision"`
		SystemUser             string      `json:"system_user"`
		Version                string      `json:"version"`
		BinaryURL              string      `json:"binary_url"`
		VerificationType       string      `json:"verification_type"`
		VerificationSource     string      `json:"verification_source"`
		ExtractCommand         string      `json:"extract_command"`
		ExcludeFiles           []string    `json:"exclude_files"`
		ExecCommandTemplate    string      `json:"exec_command_template"`
		LogrotateFilesTemplate string      `json:"logrotate_files_template"`
		PostinstScriptTemplate string      `json:"postinst_script_template"`
		ServiceType            string      `json:"service_type"`
		ProtectMemory          bool        `json:"protect_memory"`
		Mainnet                bool        `json:"mainnet"`
		ConfigFile             string      `json:"config_file"`
		AdditionalParams       interface{} `json:"additional_params"`
	} `json:"backend"`
	Blockbook struct {
		PackageName             string `json:"package_name"`
		SystemUser              string `json:"system_user"`
		InternalBindingTemplate string `json:"internal_binding_template"`
		PublicBindingTemplate   string `json:"public_binding_template"`
		ExplorerURL             string `json:"explorer_url"`
		AdditionalParams        string `json:"additional_params"`
	} `json:"blockbook"`
}

func jsonToString(msg json.RawMessage) (string, error) {
	d, err := msg.MarshalJSON()
	if err != nil {
		return "", err
	}
	return string(d), nil
}

func (c *Config) ParseTemplate() *template.Template {
	templates := map[string]string{
		"BlockChain.RPCURLTemplate":              c.BlockChain.RPCURLTemplate,
		"BlockChain.MessageQueueBindingTemplate": c.BlockChain.MessageQueueBindingTemplate,
		"Backend.ExecCommandTemplate":            c.Backend.ExecCommandTemplate,
		"Backend.LogrotateFilesTemplate":         c.Backend.LogrotateFilesTemplate,
		"Backend.PostinstScriptTemplate":         c.Backend.PostinstScriptTemplate,
		"Blockbook.InternalBindingTemplate":      c.Blockbook.InternalBindingTemplate,
		"Blockbook.PublicBindingTemplate":        c.Blockbook.PublicBindingTemplate,
	}

	funcMap := template.FuncMap{
		"jsonToString": jsonToString,
	}

	t := template.New("").Funcs(funcMap)

	for name, def := range templates {
		t = template.Must(t.Parse(fmt.Sprintf(`{{define "%s"}}%s{{end}}`, name, def)))
	}

	return t
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s coin_alias [...]\n", filepath.Base(os.Args[0]))
		os.Exit(1)
	}

	for _, coin := range os.Args[1:] {
		config := loadConfig(coin)
		generatePackageDefinitions(config)
	}
}

func loadConfig(coin string) *Config {
	dir := "configs"
	config := new(Config)

	f, err := os.Open(filepath.Join(dir, "coins", coin+".json"))
	if err != nil {
		panic(err)
	}
	d := json.NewDecoder(f)
	err = d.Decode(config)
	if err != nil {
		panic(err)
	}

	f, err = os.Open(filepath.Join(dir, "environ.json"))
	if err != nil {
		panic(err)
	}
	d = json.NewDecoder(f)
	err = d.Decode(&config.Env)
	if err != nil {
		panic(err)
	}

	config.Meta.BuildDatetime = time.Now().Format("Mon, 02 Jan 2006 15:04:05 -0700")

	switch config.Backend.ServiceType {
	case "forking":
	case "simple":
	default:
		panic("Invalid service type: " + config.Backend.ServiceType)
	}

	switch config.Backend.VerificationType {
	case "":
	case "gpg":
	case "sha256":
	case "gpg-sha256":
	default:
		panic("Invalid verification type: " + config.Backend.VerificationType)
	}

	return config
}

func generatePackageDefinitions(config *Config) {
	templ := config.ParseTemplate()

	makeOutputDir(outputDir)

	for _, subdir := range []string{"backend", "blockbook"} {
		root := filepath.Join(inputDir, subdir)

		err := os.Mkdir(filepath.Join(outputDir, subdir), 0755)
		if err != nil {
			panic(err)
		}

		err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return fmt.Errorf("%s: %s", path, err)
			}

			if path == root {
				return nil
			}
			if filepath.Base(path)[0] == '.' {
				return nil
			}

			subpath := path[len(root)-len(subdir):]

			if info.IsDir() {
				err = os.Mkdir(filepath.Join(outputDir, subpath), info.Mode())
				if err != nil {
					return fmt.Errorf("%s: %s", path, err)
				}
				return nil
			}

			t := template.Must(templ.Clone())
			t = template.Must(t.ParseFiles(path))

			err = writeTemplate(filepath.Join(outputDir, subpath), info, t, config)
			if err != nil {
				return fmt.Errorf("%s: %s", path, err)
			}

			return nil
		})
		if err != nil {
			panic(err)
		}
	}

	writeBackendConfigFile(config)
}

func makeOutputDir(path string) {
	err := os.RemoveAll(path)
	if err == nil {
		err = os.Mkdir(path, 0755)
	}
	if err != nil {
		panic(err)
	}
}

func writeTemplate(path string, info os.FileInfo, templ *template.Template, config *Config) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, info.Mode())
	if err != nil {
		return err
	}
	defer f.Close()

	err = templ.ExecuteTemplate(f, "main", config)
	if err != nil {
		return err
	}

	return nil
}

func writeBackendConfigFile(config *Config) {
	out, err := os.OpenFile(filepath.Join(outputDir, "backend/backend.conf"), os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		panic(err)
	}
	defer out.Close()

	if config.Backend.ConfigFile == "" {
		return
	} else {
		in, err := os.Open(filepath.Join(outputDir, "backend/config", config.Backend.ConfigFile))
		if err != nil {
			panic(err)
		}
		defer in.Close()

		_, err = io.Copy(out, in)
		if err != nil {
			panic(err)
		}
	}
}
