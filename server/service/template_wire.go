package service

import (
	"context"

	templatepkg "kvm_console/service/template"
)

// ── Type aliases for template subpackage ──

type TemplateMeta = templatepkg.TemplateMeta
type TemplateDefaultConfig = templatepkg.TemplateDefaultConfig
type TemplateInfo = templatepkg.TemplateInfo
type TemplateRelatedVM = templatepkg.TemplateRelatedVM
type LinuxTemplatePrepareCheck = templatepkg.LinuxTemplatePrepareCheck
type PrepareTemplateParams = templatepkg.PrepareTemplateParams
type DeleteTemplateParams = templatepkg.DeleteTemplateParams
type DeleteTemplateResult = templatepkg.DeleteTemplateResult
type DeleteTemplatePreview = templatepkg.DeleteTemplatePreview
type UpdateTemplatePublishParams = templatepkg.UpdateTemplatePublishParams
type UpdateTemplateMetaParams = templatepkg.UpdateTemplateMetaParams
type TemplateFileHash = templatepkg.TemplateFileHash
type ExportTemplateParams = templatepkg.ExportTemplateParams
type TemplateDownloadLink = templatepkg.TemplateDownloadLink
type ExportTemplateResult = templatepkg.ExportTemplateResult
type ImportTemplateParams = templatepkg.ImportTemplateParams
type ImportTemplateResult = templatepkg.ImportTemplateResult
type TemplatePackageManifest = templatepkg.TemplatePackageManifest
type TemplatePackageNode = templatepkg.TemplatePackageNode
type ImportTemplatePreviewNode = templatepkg.ImportTemplatePreviewNode
type ImportTemplatePreviewResult = templatepkg.ImportTemplatePreviewResult

// ── Constant re-exports ──

const (
	TemplateDeleteModeCascade    = templatepkg.TemplateDeleteModeCascade
	TemplateDeleteModePromote    = templatepkg.TemplateDeleteModePromote
	TemplateDeleteModePromoteHot = templatepkg.TemplateDeleteModePromoteHot
)

// ── Exported delegates (used by handler and other service files) ──

// ListTemplates delegates to templatepkg.ListTemplates
func ListTemplates() ([]TemplateInfo, error) {
	return templatepkg.ListTemplates()
}

// GetTemplateMeta delegates to templatepkg.GetTemplateMeta
func GetTemplateMeta(templateName string) *TemplateMeta {
	return templatepkg.GetTemplateMeta(templateName)
}

// GetTemplateInfoByName delegates to templatepkg.GetTemplateInfoByName
func GetTemplateInfoByName(templateName string) (*TemplateInfo, error) {
	return templatepkg.GetTemplateInfoByName(templateName)
}

// GetTemplateInfoByNodeID delegates to templatepkg.GetTemplateInfoByNodeID
func GetTemplateInfoByNodeID(nodeID string) (*TemplateInfo, error) {
	return templatepkg.GetTemplateInfoByNodeID(nodeID)
}

// PrepareTemplate delegates to templatepkg.PrepareTemplate
func PrepareTemplate(params *PrepareTemplateParams, progressFn func(int, string)) error {
	return templatepkg.PrepareTemplate(params, progressFn)
}

// PrepareImportedLinuxTemplate 为已导入 Linux 模板补齐离线克隆依赖。
func PrepareImportedLinuxTemplate(templateName string, progressFn func(int, string)) error {
	return templatepkg.PrepareImportedLinuxTemplate(templateName, progressFn)
}

// GetLinuxTemplatePrepareCheck delegates to templatepkg.GetLinuxTemplatePrepareCheck.
func GetLinuxTemplatePrepareCheck(templateName string) (*LinuxTemplatePrepareCheck, error) {
	return templatepkg.GetLinuxTemplatePrepareCheck(templateName)
}

// DeleteTemplate delegates to templatepkg.DeleteTemplate
func DeleteTemplate(templateName string) error {
	return templatepkg.DeleteTemplate(templateName)
}

// DeleteTemplateWithVMs delegates to templatepkg.DeleteTemplateWithVMs
func DeleteTemplateWithVMs(params *DeleteTemplateParams, progressFn func(int, string)) (*DeleteTemplateResult, error) {
	return templatepkg.DeleteTemplateWithVMs(params, progressFn)
}

// GetDeleteTemplatePreview delegates to templatepkg.GetDeleteTemplatePreview
func GetDeleteTemplatePreview(templateName string) (*DeleteTemplatePreview, error) {
	return templatepkg.GetDeleteTemplatePreview(templateName)
}

// ListTemplateVMs delegates to templatepkg.ListTemplateVMs
func ListTemplateVMs(templateName string) ([]TemplateRelatedVM, error) {
	return templatepkg.ListTemplateVMs(templateName)
}

// ListTemplateSubtreeVMs delegates to templatepkg.ListTemplateSubtreeVMs
func ListTemplateSubtreeVMs(templateName string) ([]TemplateRelatedVM, error) {
	return templatepkg.ListTemplateSubtreeVMs(templateName)
}

// UpdateTemplatePublish delegates to templatepkg.UpdateTemplatePublish
func UpdateTemplatePublish(templateName string, params *UpdateTemplatePublishParams) error {
	return templatepkg.UpdateTemplatePublish(templateName, params)
}

// UpdateTemplateMeta delegates to templatepkg.UpdateTemplateMeta
func UpdateTemplateMeta(templateName string, params *UpdateTemplateMetaParams) error {
	return templatepkg.UpdateTemplateMeta(templateName, params)
}

// NormalizeTemplateBootType delegates to templatepkg.NormalizeTemplateBootType
func NormalizeTemplateBootType(bootType string) string {
	return templatepkg.NormalizeTemplateBootType(bootType)
}

// DetectTemplateBootType delegates to templatepkg.DetectTemplateBootType
func DetectTemplateBootType(templatePath string) string {
	return templatepkg.DetectTemplateBootType(templatePath)
}

// DetectVMBootType delegates to templatepkg.DetectVMBootType
func DetectVMBootType(vmName string) string {
	return templatepkg.DetectVMBootType(vmName)
}

// DetectVMNVRAMPath delegates to templatepkg.DetectVMNVRAMPath
func DetectVMNVRAMPath(vmName string) string {
	return templatepkg.DetectVMNVRAMPath(vmName)
}

// ExtractDomainNVRAMPath delegates to templatepkg.ExtractDomainNVRAMPath
func ExtractDomainNVRAMPath(xmlContent string) string {
	return templatepkg.ExtractDomainNVRAMPath(xmlContent)
}

// ResolveTemplateBootType delegates to templatepkg.ResolveTemplateBootType
func ResolveTemplateBootType(templatePath, templateType, bootType string, bootVerified bool, detector func(string) string) (string, bool) {
	return templatepkg.ResolveTemplateBootType(templatePath, templateType, bootType, bootVerified, detector)
}

// EnsureTemplateVisibleForClone delegates to templatepkg.EnsureTemplateVisibleForClone
func EnsureTemplateVisibleForClone(templateName string, isAdmin bool) error {
	return templatepkg.EnsureTemplateVisibleForClone(templateName, isAdmin)
}

// GetTemplateMinDiskSizeGB delegates to templatepkg.GetTemplateMinDiskSizeGB
func GetTemplateMinDiskSizeGB(templateName string) (int, error) {
	return templatepkg.GetTemplateMinDiskSizeGB(templateName)
}

// NormalizeRequestedDiskSize delegates to templatepkg.NormalizeRequestedDiskSize
func NormalizeRequestedDiskSize(requestedDiskSize, minDiskSize int) int {
	return templatepkg.NormalizeRequestedDiskSize(requestedDiskSize, minDiskSize)
}

// ResolveCloneDiskSizeGB delegates to templatepkg.ResolveCloneDiskSizeGB
func ResolveCloneDiskSizeGB(templateName string, requestedDiskSize int) (int, error) {
	return templatepkg.ResolveCloneDiskSizeGB(templateName, requestedDiskSize)
}

// ValidateTemplateCategory delegates to templatepkg.ValidateTemplateCategory
func ValidateTemplateCategory(templateType, category string) error {
	return templatepkg.ValidateTemplateCategory(templateType, category)
}

// CalculateFileHashes delegates to templatepkg.CalculateFileHashes
func CalculateFileHashes(path string) (*TemplateFileHash, error) {
	return templatepkg.CalculateFileHashes(path)
}

// VerifyTemplateFileIntegrity delegates to templatepkg.VerifyTemplateFileIntegrity
func VerifyTemplateFileIntegrity(tpl TemplateInfo) error {
	return templatepkg.VerifyTemplateFileIntegrity(tpl)
}

// WriteVMTemplateSource delegates to templatepkg.WriteVMTemplateSource
func WriteVMTemplateSource(vmName, templateName, cloneMode string) error {
	return templatepkg.WriteVMTemplateSource(vmName, templateName, cloneMode)
}

// ReadVMTemplateSource is not delegated because vmTemplateSource is unexported in the template subpackage.
// Callers that need this should import the template subpackage directly, but currently
// no handler or external service file calls this function.

// EnsureTemplatePath delegates to templatepkg.EnsureTemplatePath
func EnsureTemplatePath(templateName string) (string, error) {
	return templatepkg.EnsureTemplatePath(templateName)
}

// ── Transfer delegates ──

// ValidateTemplateName delegates to templatepkg.ValidateTemplateName
func ValidateTemplateName(name string) error {
	return templatepkg.ValidateTemplateName(name)
}

// ValidateTemplateImportName delegates to templatepkg.ValidateTemplateImportName
func ValidateTemplateImportName(name string) error {
	return templatepkg.ValidateTemplateImportName(name)
}

// GetTemplateImportTempDir delegates to templatepkg.GetTemplateImportTempDir
func GetTemplateImportTempDir() string {
	return templatepkg.GetTemplateImportTempDir()
}

// GetTemplateExportDir delegates to templatepkg.GetTemplateExportDir
func GetTemplateExportDir() string {
	return templatepkg.GetTemplateExportDir()
}

// GetTemplateExportFileName delegates to templatepkg.GetTemplateExportFileName
func GetTemplateExportFileName(templateName string) string {
	return templatepkg.GetTemplateExportFileName(templateName)
}

// GetTemplateExportMetaFileName delegates to templatepkg.GetTemplateExportMetaFileName
func GetTemplateExportMetaFileName(templateName string) string {
	return templatepkg.GetTemplateExportMetaFileName(templateName)
}

// GetTemplateExportFilePath delegates to templatepkg.GetTemplateExportFilePath
func GetTemplateExportFilePath(templateName string) string {
	return templatepkg.GetTemplateExportFilePath(templateName)
}

// GetTemplateExportMetaFilePath delegates to templatepkg.GetTemplateExportMetaFilePath
func GetTemplateExportMetaFilePath(templateName string) string {
	return templatepkg.GetTemplateExportMetaFilePath(templateName)
}

// HasExportedTemplate delegates to templatepkg.HasExportedTemplate
func HasExportedTemplate(templateName string) bool {
	return templatepkg.HasExportedTemplate(templateName)
}

// DeleteExportedTemplate delegates to templatepkg.DeleteExportedTemplate
func DeleteExportedTemplate(templateName string) error {
	return templatepkg.DeleteExportedTemplate(templateName)
}

// ExportTemplate delegates to templatepkg.ExportTemplate
func ExportTemplate(ctx context.Context, params *ExportTemplateParams, progressFn func(int, string)) (*ExportTemplateResult, error) {
	return templatepkg.ExportTemplate(ctx, params, progressFn)
}

// PreviewImportTemplate delegates to templatepkg.PreviewImportTemplate
func PreviewImportTemplate(ctx context.Context, params *ImportTemplateParams) (*ImportTemplatePreviewResult, error) {
	return templatepkg.PreviewImportTemplate(ctx, params)
}

// ResolveImportPreviewToken delegates to templatepkg.ResolveImportPreviewToken
func ResolveImportPreviewToken(token string) (*ImportTemplateParams, error) {
	return templatepkg.ResolveImportPreviewToken(token)
}

// ImportTemplate delegates to templatepkg.ImportTemplate
func ImportTemplate(ctx context.Context, params *ImportTemplateParams, progressFn func(int, string)) (*ImportTemplateResult, error) {
	return templatepkg.ImportTemplate(ctx, params, progressFn)
}
