package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"catcube/internal/kernel/db"
	"catcube/internal/kernel/obs"
	"catcube/internal/modules/verify/contract"
	"catcube/internal/modules/verify/internal/domain"
	"catcube/internal/modules/verify/internal/infra"
	"catcube/pkg/ids"
)

// 扩展领域错误（映射到 10xxxx）。
var (
	ErrMaterialsRequired = errors.New("verify: 该认证类型需上传材料（每类最多 3 项）")
	ErrNoPermission      = errors.New("verify: 无审核权限")
	ErrCertNotPending    = errors.New("verify: 认证不在待审核状态")
	ErrAffNotFound       = errors.New("verify: 归属记录不存在或已处理")
	ErrAffNotVerified    = errors.New("verify: 仅已核验的归属可设为主单位")
	ErrOrgAdminExists    = errors.New("verify: 该成员已是组织管理员")
	ErrInviteNotFound    = errors.New("verify: 邀请不存在或已确认")
	ErrOrgIDRequired     = errors.New("verify: 缺少目标组织")
	ErrRealnameRequired  = errors.New("verify: 请先完成实名认证")
	ErrOrgNotReady       = errors.New("verify: 组织不存在、已停用或尚未通过认证")
	ErrAffAlreadyExists  = errors.New("verify: 该用户已是组织成员")
	ErrCertPending       = errors.New("verify: 实名认证审核中，请勿重复提交")
	ErrIDCardMaterials   = errors.New("verify: 请上传身份证正反面两张图片")
)

// maxMaterials 每类认证/核验最多材料数（PRD 8.4）。
const maxMaterials = 3

// CertifyFor 提交非实名类认证（2~5）：材料必填、进入平台/企业待审。
func (s *Service) CertifyFor(ctx context.Context, userID int64, in contract.CertifyInput) (contract.Cert, error) {
	switch in.CertType {
	case contract.CertPersonal, contract.CertPersonEnterprise, contract.CertEnterprise, contract.CertInstitution:
	default:
		return contract.Cert{}, domain.ErrUnsupportedCertType
	}
	if !in.Agreed {
		return contract.Cert{}, domain.ErrAgreementRequired
	}
	if len(in.MaterialFileIDs) == 0 || len(in.MaterialFileIDs) > maxMaterials {
		return contract.Cert{}, ErrMaterialsRequired
	}
	reviewerType := contract.ReviewerPlatform
	if in.CertType == contract.CertPersonEnterprise {
		if in.TargetOrgID == 0 {
			return contract.Cert{}, ErrOrgIDRequired
		}
		reviewerType = contract.ReviewerOrg
	}
	var extra []byte
	if len(in.Extra) > 0 {
		raw, _ := jsonMarshal(in.Extra)
		extra = raw
	}
	// 状态机 #3：仅「已驳回」记录重提时才走 rejected→pending 转移（首次提交无需转移）
	if latest, err := s.repo.LatestOfType(ctx, userID, in.CertType); err == nil && latest.Status == contract.CertRejected {
		if _, err := s.engine.Apply(ctx, domain.MachineCert, latest.ID, domain.CertStateRejected, domain.CertEventSubmit, "user"); err != nil {
			return contract.Cert{}, err
		}
	}
	rec := infra.Cert{ID: int64(ids.New()), UserID: userID, CertType: in.CertType,
		TargetOrgID: in.TargetOrgID, MaterialIDs: in.MaterialFileIDs, Status: contract.CertPending}
	if err := s.pool.InTx(ctx, func(tx db.Tx) error {
		return s.repo.InsertPending(ctx, tx, rec, reviewerType, extra)
	}); err != nil {
		return contract.Cert{}, err
	}
	return toContract(rec), nil
}

// ReviewCert 审核认证：平台（1~5 类）或组织管理员（个人企业身份）。
func (s *Service) ReviewCert(ctx context.Context, certID, reviewerID int64, reviewerType int16, pass bool, reason string) error {
	c, err := s.repo.GetCert(ctx, certID)
	if err != nil {
		return err
	}
	if c.Status != contract.CertPending {
		return ErrCertNotPending
	}
	if reviewerType == contract.ReviewerOrg {
		if c.ReviewerType != contract.ReviewerOrg && c.CertType != contract.CertPersonEnterprise {
			return ErrNoPermission
		}
		ok, err := s.repo.ActiveOrgAdmin(ctx, c.TargetOrgID, reviewerID)
		if err != nil || !ok {
			return ErrNoPermission
		}
	}
	status := contract.CertRejected
	event := domain.CertEventReject
	if pass {
		status = contract.CertApproved
		event = domain.CertEventApprove
		reason = ""
	}
	// 状态机 #3：待审核 → 已通过/已驳回
	if _, err := s.engine.Apply(ctx, domain.MachineCert, certID, domain.CertStatePending, event, "reviewer"); err != nil {
		return err
	}
	if err := s.pool.InTx(ctx, func(tx db.Tx) error {
		if _, err := s.repo.DecideCert(ctx, tx, certID, reviewerID, status, reason); err != nil {
			return err
		}
		if !pass {
			if s.bus == nil {
				return nil
			}
			payload := contract.CertificationPayload{UserID: c.UserID, CertType: c.CertType, Reason: reason}
			return s.bus.Publish(ctx, tx, contract.NewCertificationRejected(ids.New().String(), payload))
		}
		// 通过：授 Badge（实名→realname；2~5→verified）
		badge := contract.BadgeVerified
		if c.CertType == contract.CertRealname {
			badge = contract.BadgeRealname
		}
		subject := "user"
		subjectID := c.UserID
		if c.CertType == contract.CertEnterprise || c.CertType == contract.CertInstitution {
			// 企业/机构认证：标识打在组织实体上（以申请人账号为组织主体存储，一期一致）
			subject = "org"
			subjectID = c.UserID
		}
		if err := s.repo.GrantBadge(ctx, tx, subject, subjectID, badge); err != nil {
			return err
		}
		// 企业/机构认证通过：申请人成为组织管理员（首任）
		if c.CertType == contract.CertEnterprise || c.CertType == contract.CertInstitution {
			if err := s.repo.UpsertActiveOrgAdmin(ctx, tx, int64(ids.New()), c.UserID, c.UserID); err != nil {
				return err
			}
		}
		if s.bus == nil {
			return nil
		}
		payload := contract.CertificationPayload{UserID: c.UserID, CertType: c.CertType, BadgeCode: badge}
		return s.bus.Publish(ctx, tx, contract.NewCertificationApproved(ids.New().String(), payload))
	}); err != nil {
		return err
	}
	if c.CertType == contract.CertRealname {
		rs := int16(2)
		if pass {
			rs = 1
		}
		if err := s.identity.MarkRealname(ctx, c.UserID, rs); err != nil {
			return err
		}
	}
	obs.Default().Inc("catcube_cert_reviewed_total")
	return nil
}

// PendingCerts 待审列表（平台/组织视角）。
func (s *Service) PendingCerts(ctx context.Context, reviewerType int16, orgID int64, limit int) ([]contract.Cert, error) {
	rows, err := s.repo.PendingCerts(ctx, reviewerType, orgID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Cert, 0, len(rows))
	for _, c := range rows {
		out = append(out, toContract(c))
	}
	return out, nil
}

// ApplyAffiliation 申请加入组织（一人多单位）。
func (s *Service) ApplyAffiliation(ctx context.Context, userID, orgID int64, role string, proofFileIDs []int64) (contract.Affiliation, error) {
	if orgID == 0 {
		return contract.Affiliation{}, ErrOrgIDRequired
	}
	if len(proofFileIDs) > maxMaterials {
		return contract.Affiliation{}, ErrMaterialsRequired
	}
	if ok, err := s.IsRealname(ctx, userID); err != nil || !ok {
		return contract.Affiliation{}, ErrRealnameRequired
	}
	if err := s.requireOrgReady(ctx, orgID); err != nil {
		return contract.Affiliation{}, err
	}
	// 状态机 #4：仅已驳回的归属重新申请时才走 rejected→pending 转移
	if prev, err := s.repo.GetAffiliationByUserOrg(ctx, userID, orgID); err == nil && prev.Status == contract.AffRejected {
		if _, err := s.engine.Apply(ctx, domain.MachineAffiliation, prev.ID, domain.AffStateRejected, domain.AffEventApply, "user"); err != nil {
			return contract.Affiliation{}, err
		}
	}
	a := infra.Affiliation{ID: int64(ids.New()), UserID: userID, OrgID: orgID, Role: role, ProofIDs: proofFileIDs}
	if err := s.pool.InTx(ctx, func(tx db.Tx) error {
		if err := s.repo.InsertAffiliation(ctx, tx, a); err != nil {
			return err
		}
		if s.bus == nil {
			return nil
		}
		return s.bus.Publish(ctx, tx, contract.NewOrgAdminChanged(ids.New().String(),
			contract.OrgAdminPayload{OrgID: orgID, UserID: userID, Action: "affiliation_applied"}))
	}); err != nil {
		return contract.Affiliation{}, err
	}
	return toAffiliation(a), nil
}

// AffiliationsOfOrg 组织成员列表（后台组织管理）。
func (s *Service) AffiliationsOfOrg(ctx context.Context, orgID int64) ([]contract.Affiliation, error) {
	rows, err := s.repo.AffiliationsOfOrg(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Affiliation, 0, len(rows))
	for _, a := range rows {
		out = append(out, toAffiliation(a))
	}
	return out, nil
}

// AssistAffiliation 管理端辅助加入组织：成员须已实名；运营方直接核验通过。
func (s *Service) AssistAffiliation(ctx context.Context, orgID, userID int64, role string, proofFileIDs []int64, operatorID int64) (contract.Affiliation, error) {
	if orgID == 0 || userID == 0 {
		return contract.Affiliation{}, ErrOrgIDRequired
	}
	if len(proofFileIDs) > maxMaterials {
		return contract.Affiliation{}, ErrMaterialsRequired
	}
	if ok, err := s.IsRealname(ctx, userID); err != nil || !ok {
		return contract.Affiliation{}, ErrRealnameRequired
	}
	b, err := s.identity.UserBrief(ctx, orgID)
	if err != nil || b.Status != 0 {
		return contract.Affiliation{}, ErrOrgNotReady
	}
	a := infra.Affiliation{ID: int64(ids.New()), UserID: userID, OrgID: orgID, Role: role, ProofIDs: proofFileIDs, Status: contract.AffVerified}
	if err := s.pool.InTx(ctx, func(tx db.Tx) error {
		if err := s.repo.InsertVerifiedAffiliation(ctx, tx, a, operatorID); err != nil {
			return err
		}
		if s.bus == nil {
			return nil
		}
		payload := contract.AffiliationPayload{AffiliationID: a.ID, UserID: userID, OrgID: orgID, Status: contract.AffVerified}
		return s.bus.Publish(ctx, tx, contract.NewAffiliationVerified(ids.New().String(), payload))
	}); err != nil {
		return contract.Affiliation{}, err
	}
	return toAffiliation(a), nil
}

// RemoveAffiliation 管理端移除组织成员。
func (s *Service) RemoveAffiliation(ctx context.Context, orgID, userID, operatorID int64) error {
	ok, err := s.repo.RemoveAffiliation(ctx, orgID, userID, operatorID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrAffNotFound
	}
	return nil
}

// requireOrgReady 组织必须存在且已通过企业/机构认证（平台侧辅助加入不强制认证）。
func (s *Service) requireOrgReady(ctx context.Context, orgID int64) error {
	b, err := s.identity.UserBrief(ctx, orgID)
	if err != nil || b.Status != 0 {
		return ErrOrgNotReady
	}
	for _, t := range []int16{contract.CertEnterprise, contract.CertInstitution} {
		if c, err := s.repo.LatestOfType(ctx, orgID, t); err == nil && c.Status == contract.CertApproved {
			return nil
		}
	}
	return ErrOrgNotReady
}

// AffiliationsOf 归属列表（一人多单位：置灰/红框由 status 驱动）。
func (s *Service) AffiliationsOf(ctx context.Context, userID int64) ([]contract.Affiliation, error) {
	rows, err := s.repo.AffiliationsOf(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Affiliation, 0, len(rows))
	for _, a := range rows {
		out = append(out, toAffiliation(a))
	}
	return out, nil
}

// VerifyAffiliation 核验归属（组织管理员；先到先处理——首个处理者生效，其余因状态已变更被拒）。
func (s *Service) VerifyAffiliation(ctx context.Context, affID, reviewerID int64, pass bool, reason string) error {
	a, err := s.repo.GetAffiliation(ctx, affID)
	if err != nil {
		return ErrAffNotFound
	}
	ok, err := s.repo.ActiveOrgAdmin(ctx, a.OrgID, reviewerID)
	if err != nil || !ok {
		return ErrNoPermission
	}
	status := contract.AffRejected
	event := domain.AffEventReject
	if pass {
		status = contract.AffVerified
		event = domain.AffEventVerify
		reason = ""
	}
	// 状态机 #4：待核验 → 已核验/已驳回
	if _, err := s.engine.Apply(ctx, domain.MachineAffiliation, affID, domain.AffStatePending, event, "org_admin"); err != nil {
		return err
	}
	var decided bool
	if err := s.pool.InTx(ctx, func(tx db.Tx) error {
		var err error
		decided, err = s.repo.DecideAffiliation(ctx, tx, affID, reviewerID, status, reason)
		if err != nil || !decided {
			return err
		}
		if s.bus == nil {
			return nil
		}
		payload := contract.AffiliationPayload{AffiliationID: affID, UserID: a.UserID, OrgID: a.OrgID, Status: status}
		return s.bus.Publish(ctx, tx, contract.NewAffiliationVerified(ids.New().String(), payload))
	}); err != nil {
		return err
	}
	if !decided {
		return ErrAffNotFound // 已被他人处理（先到先处理）
	}
	return nil
}

// SetPrimaryAffiliation 设为主单位（唯一）。
func (s *Service) SetPrimaryAffiliation(ctx context.Context, userID, affID int64) error {
	a, err := s.repo.GetAffiliation(ctx, affID)
	if err != nil || a.UserID != userID {
		return ErrAffNotFound
	}
	if a.Status != contract.AffVerified {
		return ErrAffNotVerified
	}
	return s.pool.InTx(ctx, func(tx db.Tx) error { return s.repo.SetPrimary(ctx, tx, userID, affID) })
}

// PendingAffiliations 待核验列表（仅组织管理员可见）。
func (s *Service) PendingAffiliations(ctx context.Context, orgID, reviewerID int64, limit int) ([]contract.Affiliation, error) {
	ok, err := s.repo.ActiveOrgAdmin(ctx, orgID, reviewerID)
	if err != nil || !ok {
		return nil, ErrNoPermission
	}
	rows, err := s.repo.PendingAffiliations(ctx, orgID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Affiliation, 0, len(rows))
	for _, a := range rows {
		out = append(out, toAffiliation(a))
	}
	return out, nil
}

// OverdueAffiliations 超期未核验归属（平台代处理队列；默认 >7 天）。
func (s *Service) OverdueAffiliations(ctx context.Context, days, limit int) ([]contract.Affiliation, error) {
	if days <= 0 {
		days = 7
	}
	rows, err := s.repo.OverdueAffiliations(ctx, days, limit)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Affiliation, 0, len(rows))
	for _, a := range rows {
		out = append(out, toAffiliation(a))
	}
	return out, nil
}

// ResolveAffiliation 平台代处理（组织管理员超 7 天未处理）。
func (s *Service) ResolveAffiliation(ctx context.Context, affID, operatorID int64, pass bool, reason string) error {
	status := contract.AffRejected
	event := domain.AffEventReject
	if pass {
		status = contract.AffVerified
		event = domain.AffEventVerify
		reason = "平台代处理"
	}
	if _, err := s.engine.Apply(ctx, domain.MachineAffiliation, affID, domain.AffStatePending, event, "platform"); err != nil {
		return err
	}
	var decided bool
	if err := s.pool.InTx(ctx, func(tx db.Tx) error {
		var err error
		decided, err = s.repo.DecideAffiliation(ctx, tx, affID, operatorID, status, reason)
		return err
	}); err != nil {
		return err
	}
	if !decided {
		return ErrAffNotFound
	}
	obs.Default().Inc("catcube_affiliation_resolved_total")
	return nil
}

// InviteOrgAdmin 邀请组织管理员（需对方确认）。
func (s *Service) InviteOrgAdmin(ctx context.Context, orgID, inviterID, inviteeID int64) error {
	ok, err := s.repo.ActiveOrgAdmin(ctx, orgID, inviterID)
	if err != nil || !ok {
		return ErrNoPermission
	}
	if inviteeID == 0 {
		return ErrOrgIDRequired
	}
	return s.pool.InTx(ctx, func(tx db.Tx) error {
		return s.repo.InsertOrgAdmin(ctx, tx, int64(ids.New()), orgID, inviterID, inviteeID)
	})
}

// ConfirmOrgAdmin 被邀请人确认。
func (s *Service) ConfirmOrgAdmin(ctx context.Context, inviteID, userID int64) error {
	var ok bool
	if err := s.pool.InTx(ctx, func(tx db.Tx) error {
		var err error
		ok, err = s.repo.ConfirmOrgAdmin(ctx, tx, inviteID, userID)
		return err
	}); err != nil {
		return err
	}
	if !ok {
		return ErrInviteNotFound
	}
	return nil
}

// OrgAdminsOf 管理员列表。
func (s *Service) OrgAdminsOf(ctx context.Context, orgID int64) ([]contract.OrgAdmin, error) {
	rows, err := s.repo.OrgAdminsOf(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]contract.OrgAdmin, 0, len(rows))
	for _, a := range rows {
		out = append(out, contract.OrgAdmin{OrgID: a.OrgID, UserID: a.UserID, Status: a.Status})
	}
	return out, nil
}

// ResetOrgAdmin 平台重置（移除失联管理员/直接指派）。
func (s *Service) ResetOrgAdmin(ctx context.Context, orgID, operatorID, userID int64, remove bool) error {
	return s.pool.InTx(ctx, func(tx db.Tx) error {
		if remove {
			return s.repo.RemoveOrgAdmin(ctx, tx, orgID, userID, fmt.Sprintf("平台重置（操作者 %d）", operatorID))
		}
		return s.repo.UpsertActiveOrgAdmin(ctx, tx, int64(ids.New()), orgID, userID)
	})
}

// IsOrgAdmin 是否在任组织管理员。
func (s *Service) IsOrgAdmin(ctx context.Context, orgID, userID int64) (bool, error) {
	return s.repo.ActiveOrgAdmin(ctx, orgID, userID)
}

func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }

func toAffiliation(a infra.Affiliation) contract.Affiliation {
	return contract.Affiliation{ID: a.ID, UserID: a.UserID, OrgID: a.OrgID, Role: a.Role,
		IsPrimary: a.IsPrimary, Status: a.Status, RejectReason: a.RejectReason}
}
