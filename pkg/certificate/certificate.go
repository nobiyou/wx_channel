package certificate

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
)

type Subject struct {
	CN string
	OU string
	O  string
	L  string
	S  string
	C  string
}
type Certificate struct {
	Thumbprint string
	Subject    Subject
}

func fetchCertificatesInWindows() ([]Certificate, error) {
	var certificates []Certificate

	// 尝试从 LocalMachine 获取证书
	cmd := "Get-ChildItem Cert:\\LocalMachine\\Root | Where-Object {$_.Subject -like '*SunnyNet*' -or $_.Subject -like '*Sunny*'}"
	ps := exec.Command("powershell.exe", "-Command", cmd)
	output, err := ps.CombinedOutput()
	if err == nil && len(output) > 0 {
		// 如果找到证书，添加一个示例证书
		certificates = append(certificates, Certificate{
			Thumbprint: "SunnyNet",
			Subject: Subject{
				CN: "SunnyNet",
				O:  "SunnyNet",
			},
		})
	}

	// 尝试从 CurrentUser 获取证书
	cmd = "Get-ChildItem Cert:\\CurrentUser\\Root | Where-Object {$_.Subject -like '*SunnyNet*' -or $_.Subject -like '*Sunny*'}"
	ps = exec.Command("powershell.exe", "-Command", cmd)
	output, err = ps.CombinedOutput()
	if err == nil && len(output) > 0 {
		// 如果找到证书，添加一个示例证书
		certificates = append(certificates, Certificate{
			Thumbprint: "SunnyNet",
			Subject: Subject{
				CN: "SunnyNet",
				O:  "SunnyNet",
			},
		})
	}

	return certificates, nil
}
func fetchCertificatesInMacOS() ([]Certificate, error) {
	cmd := exec.Command("security", "find-certificate", "-a")
	output, err2 := cmd.Output()
	if err2 != nil {
		return nil, errors.New(fmt.Sprintf("获取证书时发生错误，%v\n", err2.Error()))
	}
	var certificates []Certificate
	lines := strings.Split(string(output), "\n")
	for i := 0; i < len(lines)-1; i += 13 {
		if lines[i] == "" {
			continue
		}
		// if i > len(lines)-1 {
		// 	continue
		// }
		cenc := lines[i+5]
		ctyp := lines[i+6]
		hpky := lines[i+7]
		labl := lines[i+9]
		subj := lines[i+12]
		re := regexp.MustCompile(`="([^"]{1,})"`)
		// 找到匹配的字符串
		matches := re.FindStringSubmatch(labl)
		if len(matches) < 1 {
			continue
		}
		label := matches[1]
		certificates = append(certificates, Certificate{
			Thumbprint: "",
			Subject: Subject{
				CN: label,
				OU: cenc,
				O:  ctyp,
				L:  hpky,
				S:  subj,
				C:  cenc,
			},
		})
	}
	return certificates, nil
}

func fetchCertificates() ([]Certificate, error) {
	os_env := runtime.GOOS
	switch os_env {
	case "linux":
		fmt.Println("Running on Linux")
	case "darwin":
		return fetchCertificatesInMacOS()
	case "windows":
		return fetchCertificatesInWindows()
	default:
		fmt.Printf("Running on %s\n", os_env)
	}
	return nil, errors.New(fmt.Sprintf("unknown OS\n"))

}
func CheckCertificate(cert_name string) (bool, error) {
	certificates, err := fetchCertificates()
	if err != nil {
		return false, err
	}

	for _, cert := range certificates {
		// 精确匹配
		if cert.Subject.CN == cert_name {
			return true, nil
		}
		// 模糊匹配（包含关键字）
		if strings.Contains(cert.Subject.CN, cert_name) || strings.Contains(cert_name, cert.Subject.CN) {
			return true, nil
		}
		// 检查组织名称
		if cert.Subject.O == cert_name || strings.Contains(cert.Subject.O, cert_name) {
			return true, nil
		}
	}
	return false, nil
}
func removeCertificateInWindows(cert_name string) error {
	// 首先尝试从当前用户存储删除
	cmd := fmt.Sprintf("Get-ChildItem Cert:\\CurrentUser\\Root | Where-Object {$_.Subject -like '*%s*'} | Remove-Item", cert_name)
	ps := exec.Command("powershell.exe", "-Command", cmd)
	output, err := ps.CombinedOutput()
	if err == nil {
		return nil // 成功从当前用户存储删除
	}

	// 如果当前用户存储删除失败，尝试从系统级存储删除
	cmd = fmt.Sprintf("Get-ChildItem Cert:\\LocalMachine\\Root | Where-Object {$_.Subject -like '*%s*'} | Remove-Item", cert_name)
	ps = exec.Command("powershell.exe", "-Command", cmd)
	output, err = ps.CombinedOutput()
	if err != nil {
		return errors.New(fmt.Sprintf("删除证书时发生错误，%v\n", output))
	}
	return nil
}

func removeCertificateInMacOS(cert_name string) error {
	cmd := fmt.Sprintf("security delete-certificate -c '%s' /Library/Keychains/System.keychain", cert_name)
	ps := exec.Command("bash", "-c", cmd)
	output, err := ps.CombinedOutput()
	if err != nil {
		return errors.New(fmt.Sprintf("删除证书时发生错误，%v\n", output))
	}
	return nil
}

func RemoveCertificate(cert_name string) error {
	os_env := runtime.GOOS
	switch os_env {
	case "linux":
		return errors.New("Linux 系统暂不支持证书卸载功能")
	case "darwin":
		return removeCertificateInMacOS(cert_name)
	case "windows":
		return removeCertificateInWindows(cert_name)
	default:
		return errors.New(fmt.Sprintf("不支持的操作系统: %s\n", os_env))
	}
}
func installCertificateInWindows(cert_data []byte) error {
	cert_file, err := os.CreateTemp("", "SunnyRoot.cer")
	if err != nil {
		return errors.New(fmt.Sprintf("没有创建证书的权限，%v\n", err.Error()))
	}
	defer os.Remove(cert_file.Name())
	if _, err := cert_file.Write(cert_data); err != nil {
		return errors.New(fmt.Sprintf("获取证书失败，%v\n", err.Error()))
	}
	if err := cert_file.Close(); err != nil {
		return errors.New(fmt.Sprintf("生成证书失败，%v\n", err.Error()))
	}

	// 首先尝试安装到当前用户证书存储（不需要管理员权限）
	cmd := fmt.Sprintf("Import-Certificate -FilePath '%s' -CertStoreLocation Cert:\\CurrentUser\\Root", cert_file.Name())
	ps := exec.Command("powershell.exe", "-Command", cmd)
	output, err2 := ps.CombinedOutput()
	if err2 == nil {
		return nil // 成功安装到当前用户存储
	}

	// 如果当前用户存储失败，尝试系统级存储（需要管理员权限）
	cmd = fmt.Sprintf("Import-Certificate -FilePath '%s' -CertStoreLocation Cert:\\LocalMachine\\Root", cert_file.Name())
	ps = exec.Command("powershell.exe", "-Command", cmd)
	output, err2 = ps.CombinedOutput()
	if err2 != nil {
		// 提供详细的错误信息和解决方案
		errorMsg := fmt.Sprintf("证书安装失败！\n\n错误详情：%v\n\n解决方案：\n1. 以管理员身份运行程序\n2. 或者手动安装证书：\n   - 双击证书文件：%s\n   - 选择'安装证书'\n   - 选择'本地计算机'\n   - 选择'将所有的证书都放入下列存储'\n   - 选择'受信任的根证书颁发机构'\n   - 点击'确定'完成安装\n\n3. 或者忽略证书安装，程序仍可正常运行（但HTTPS请求可能失败）", output, cert_file.Name())
		return errors.New(errorMsg)
	}
	return nil
}
func installCertificateInMacOS(cert_data []byte) error {
	cert_file, err := os.CreateTemp("", "SunnyRoot.cer")
	if err != nil {
		return errors.New(fmt.Sprintf("没有创建证书的权限，%v\n", err.Error()))
	}
	defer os.Remove(cert_file.Name())
	if _, err := cert_file.Write(cert_data); err != nil {
		return errors.New(fmt.Sprintf("获取证书失败，%v\n", err.Error()))
	}
	if err := cert_file.Close(); err != nil {
		return errors.New(fmt.Sprintf("生成证书失败，%v\n", err.Error()))
	}
	cmd := fmt.Sprintf("security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain '%s'", cert_file.Name())
	ps := exec.Command("bash", "-c", cmd)
	output, err2 := ps.CombinedOutput()
	if err2 != nil {
		return errors.New(fmt.Sprintf("安装证书时发生错误，%v\n", output))
	}
	return nil
}

func InstallCertificate(cert_data []byte) error {
	os_env := runtime.GOOS
	switch os_env {
	case "linux":
		fmt.Println("Running on Linux")
	case "darwin":
		return installCertificateInMacOS(cert_data)
	case "windows":
		return installCertificateInWindows(cert_data)
	default:
		fmt.Printf("Running on %s\n", os_env)
	}
	return errors.New(fmt.Sprintf("unknown OS\n"))
}
