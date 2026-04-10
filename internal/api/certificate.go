package api

import (
	"net/http"
	"runtime"

	"wx_channel/internal/assets"
	"wx_channel/internal/response"
	"wx_channel/pkg/certificate"

	"github.com/qtgolang/SunnyNet/SunnyNet"
)

// certNameAndData returns the platform-appropriate cert name and data.
func certNameAndData() (string, []byte) {
	if runtime.GOOS == "darwin" {
		return "WxChannel macOS MITM CA", assets.MitmCACert
	}
	return "SunnyNet", assets.CertData
}

// CertificateService 证书服务
type CertificateService struct {
	sunny *SunnyNet.Sunny
}

// NewCertificateService 创建证书服务
func NewCertificateService(sunny *SunnyNet.Sunny) *CertificateService {
	return &CertificateService{
		sunny: sunny,
	}
}

// GetStatus 获取证书状态
func (s *CertificateService) GetStatus(w http.ResponseWriter, r *http.Request) {
	certName, _ := certNameAndData()
	installed, err := certificate.CheckCertificate(certName)
	if err != nil {
		response.Error(w, 500, "Failed to check certificate: "+err.Error())
		return
	}

	status := map[string]interface{}{
		"installed": installed,
		"name":      certName,
	}
	response.Success(w, status)
}

// Install 安装证书
func (s *CertificateService) Install(w http.ResponseWriter, r *http.Request) {
	_, certData := certNameAndData()
	err := certificate.InstallCertificate(certData)
	if err != nil {
		response.Error(w, 500, "Failed to install certificate: "+err.Error())
		return
	}

	response.Success(w, "Certificate installation started/completed")
}

// Download 下载证书
func (s *CertificateService) Download(w http.ResponseWriter, r *http.Request) {
	// 提供证书下载，方便用户手动安装
	w.Header().Set("Content-Disposition", "attachment; filename=SunnyRoot.cer")
	w.Header().Set("Content-Type", "application/x-x509-ca-cert")
	w.Write(assets.CertData)
}

// RegisterRoutes 注册路由
func (s *CertificateService) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/certificate/status", s.GetStatus)
	mux.HandleFunc("/api/v1/certificate/install", s.Install)
	mux.HandleFunc("/api/v1/certificate/download", s.Download)
}
