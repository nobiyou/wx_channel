package api

import (
	"net/http"

	"wx_channel/internal/response"
	"wx_channel/pkg/certificate"

	"github.com/qtgolang/SunnyNet/SunnyNet"
)

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
	// 检查 "SunnyNet" 证书是否安装
	installed, err := certificate.CheckCertificate("SunnyNet")
	if err != nil {
		response.Error(w, 500, "Failed to check certificate: "+err.Error())
		return
	}

	status := map[string]interface{}{
		"installed": installed,
		"name":      "SunnyNet",
	}
	response.Success(w, status)
}

// Install 安装证书
func (s *CertificateService) Install(w http.ResponseWriter, r *http.Request) {
	certData := s.sunny.ExportCert()
	if len(certData) == 0 {
		response.Error(w, http.StatusInternalServerError, "runtime certificate unavailable")
		return
	}
	err := certificate.InstallCertificate(certData)
	if err != nil {
		// 证书安装可能因为用户取消或权限不足失败
		response.Error(w, 500, "Failed to install certificate: "+err.Error())
		return
	}

	response.Success(w, "Certificate installation started/completed")
}

// Download 下载证书
func (s *CertificateService) Download(w http.ResponseWriter, r *http.Request) {
	certData := s.sunny.ExportCert()
	if len(certData) == 0 {
		response.Error(w, http.StatusInternalServerError, "runtime certificate unavailable")
		return
	}
	// 提供证书下载，方便用户手动安装
	w.Header().Set("Content-Disposition", "attachment; filename=SunnyRoot.cer")
	w.Header().Set("Content-Type", "application/x-x509-ca-cert")
	_, _ = w.Write(certData)
}

// RegisterRoutes 注册路由
func (s *CertificateService) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/certificate/status", s.GetStatus)
	mux.HandleFunc("/api/v1/certificate/install", s.Install)
	mux.HandleFunc("/api/v1/certificate/download", s.Download)
}
