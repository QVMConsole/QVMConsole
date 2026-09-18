package service

// Clone compatibility types - delegate to service/clone subpackage
import clonepkg "kvm_console/service/clone"

type CloneParams = clonepkg.CloneParams
type BatchCloneParams = clonepkg.BatchCloneParams
type ReinstallParams = clonepkg.ReinstallParams
type CloneResult = clonepkg.CloneResult
type LinkedCloneParams = clonepkg.LinkedCloneParams
type LinkedCloneResult = clonepkg.LinkedCloneResult
