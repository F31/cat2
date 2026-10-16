// Package app verify 应用服务：实名认证（实时二要素）、状态机执行、Badge 授予、结果回写。
package app

import (
	"context"
	"errors"
	"fmt"

	"catcube/internal/kernel/db"
	"catcube/internal/kernel/eventbus"
	"catcube/internal/kernel/sensitive"
	"catcube/internal/kernel/statemachine"
	idcontract "catcube/internal/modules/identity/contract"
	"catcube/internal/modules/verify/contract"
	"catcube/internal/modules/verify/internal/domain"
	"catcube/internal/modules/verify/internal/infra"
	"catcube/internal/ports"
	"catcube/internal/shared/entityref"
	"catcube/pkg/ids"
)

// Repo 应用层所需仓储能力（PG 实现见 infra.Store）。
type Repo interface {
	Insert(ctx context.Context, tx db.Tx, c infra.Cert) error
	GetCert(ctx context.Context, id int64) (infra.Cert, error)
	LatestOfType(ctx context.Context, userID int64, certType int16) (infra.Cert, error)
	InsertPending(ctx context.Context, tx db.Tx, c infra.Cert, reviewerType int16, extra []byte) error
	DecideCert(ctx context.Context, tx db.Tx, id, reviewerID int64, status int16, reason string) (bool, error)
	PendingCerts(ctx context.Context, reviewerType int16, orgID int64, limit int) ([]infra.Cert, error)
	InsertAffiliation(ctx context.Context, tx db.Tx, a infra.Affiliation) error
	GetAffiliation(ctx context.Context, id int64) (infra.Affiliation, error)
	GetAffiliationByUserOrg(ctx context.Context, userID, orgID int64) (infra.Affiliation, error)
	PendingAffiliations(ctx context.Context, orgID int64, limit int) ([]infra.Affiliation, error)
	AffiliationsOf(ctx context.Context, userID int64) ([]infra.Affiliation, error)
	DecideAffiliation(ctx context.Context, tx db.Tx, id, reviewerID int64, status int16, reason string) (bool, error)
	SetPrimary(ctx context.Context, tx db.Tx, userID, id int64) error
	InsertOrgAdmin(ctx context.Context, tx db.Tx, id, orgID, inviterID, inviteeID int64) error
	ConfirmOrgAdmin(ctx context.Context, tx db.Tx, inviteID, userID int64) (bool, error)
	UpsertActiveOrgAdmin(ctx context.Context, tx db.Tx, id, orgID, userID int64) error
	RemoveOrgAdmin(ctx context.Context, tx db.Tx, orgID, userID int64, reason string) error
	OrgAdminsOf(ctx context.Context, orgID int64) ([]infra.OrgAdmin, error)
	ActiveOrgAdmin(ctx context.Context, orgID, userID int64) (bool, error)
	LatestRealname(ctx context.Context, userID int64) (infra.Cert, error)
	CertsOf(ctx context.Context, userID int64) ([]infra.Cert, error)
	GrantBadge(ctx context.Context, tx db.Tx, subjectType string, subjectID int64, code string) error
	BadgesOf(ctx context.Context, subjectType string, subjectID int64) ([]contract.Badge, error)
}

// Options 构造参数。
type Options struct {
	Repo     Repo
	Pool     *db.Pool
	Bus      *eventbus.Bus
	Verify   ports.RealnameVerify
	Identity idcontract.Facade
	Encrypt  *sensitive.Encryptor
}

// Service 认证应用服务。
type Service struct {
	repo     Repo
	pool     *db.Pool
	bus      *eventbus.Bus
	verify   ports.RealnameVerify
	identity idcontract.Facade
	enc      *sensitive.Encryptor
	engine   *statemachine.Engine
}

func New(o Options) *Service {
	return &Service{repo: o.Repo, pool: o.Pool, bus: o.Bus, verify: o.Verify,
		identity: o.Identity, enc: o.Encrypt, engine: statemachine.NewEngine(nil)}
}

// Certify 提交认证（一期仅实名）：状态机校验 → 二要素实时核验 → 落库/授标/发事件 → 回写 identity。
func (s *Service) Certify(ctx context.Context, userID int64, in contract.CertifyInput) (contract.Cert, error) {
	if in.CertType != contract.CertRealname {
		return contract.Cert{}, domain.ErrUnsupportedCertType
	}
	if !in.Agreed {
		return contract.Cert{}, domain.ErrAgreementRequired
	}
	if !domain.ValidName(in.RealName) {
		return contract.Cert{}, domain.ErrNameRequired
	}
	if !domain.ValidIDCard(in.IDCardNo) {
		return contract.Cert{}, domain.ErrInvalidIDCard
	}

	latest, err := s.repo.LatestRealname(ctx, userID)
	has := err == nil
	if err != nil && !errors.Is(err, infra.ErrNotFound) {
		return contract.Cert{}, err
	}
	cur := domain.StateOf(latest.Status, has)
	if cur == domain.StateVerified {
		return contract.Cert{}, domain.ErrAlreadyVerified
	}

	// 进入「核验中」（未实名 submit / 失败后 retry）
	enterEvent := domain.EventSubmit
	if cur == domain.StateFailed {
		enterEvent = domain.EventRetry
	}
	if _, err := s.engine.Apply(ctx, domain.RealnameMachine, userID, cur, enterEvent, "user"); err != nil {
		return contract.Cert{}, err
	}

	matched, verr := s.verify.Verify(ctx, in.RealName, in.IDCardNo)
	if verr != nil {
		return contract.Cert{}, fmt.Errorf("%w: %v", domain.ErrUpstream, verr)
	}

	// 结束「核验中」→ 已实名 / 核验失败
	status := contract.CertRejected
	reason := domain.RejectReason
	exitEvent := domain.EventFail
	if matched {
		status = contract.CertApproved
		reason = ""
		exitEvent = domain.EventPass
	}
	if _, err := s.engine.Apply(ctx, domain.RealnameMachine, userID, domain.StateReviewing, exitEvent, "system"); err != nil {
		return contract.Cert{}, err
	}

	var nameEnc, idEnc []byte
	if s.enc != nil {
		nameEnc, _ = s.enc.Encrypt([]byte(in.RealName))
		idEnc, _ = s.enc.Encrypt([]byte(in.IDCardNo))
	}
	rec := infra.Cert{
		ID: int64(ids.New()), UserID: userID, CertType: in.CertType,
		RealNameEnc: nameEnc, IDNoEnc: idEnc, Status: status, RejectReason: reason,
	}

	if err := s.pool.InTx(ctx, func(tx db.Tx) error {
		if err := s.repo.Insert(ctx, tx, rec); err != nil {
			return err
		}
		if status == contract.CertApproved {
			if err := s.repo.GrantBadge(ctx, tx, "user", userID, contract.BadgeRealname); err != nil {
				return err
			}
		}
		if s.bus == nil {
			return nil
		}
		payload := contract.CertificationPayload{UserID: userID, CertType: in.CertType}
		if status == contract.CertApproved {
			payload.BadgeCode = contract.BadgeRealname
			return s.bus.Publish(ctx, tx, contract.NewCertificationApproved(ids.New().String(), payload))
		}
		payload.Reason = reason
		return s.bus.Publish(ctx, tx, contract.NewCertificationRejected(ids.New().String(), payload))
	}); err != nil {
		return contract.Cert{}, err
	}

	// 回写 identity（经 contract，不读 idn_user）
	rs := int16(2)
	if status == contract.CertApproved {
		rs = 1
	}
	if err := s.identity.MarkRealname(ctx, userID, rs); err != nil {
		return contract.Cert{}, err
	}
	return toContract(rec), nil
}

// CertsOf 认证记录。
func (s *Service) CertsOf(ctx context.Context, userID int64) ([]contract.Cert, error) {
	rows, err := s.repo.CertsOf(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Cert, 0, len(rows))
	for _, r := range rows {
		out = append(out, toContract(r))
	}
	return out, nil
}

// BadgesOf 某实体标识。
func (s *Service) BadgesOf(ctx context.Context, ref entityref.Ref) ([]contract.Badge, error) {
	return s.repo.BadgesOf(ctx, ref.Type, ref.ID)
}

// IsRealname 是否已实名。
func (s *Service) IsRealname(ctx context.Context, userID int64) (bool, error) {
	c, err := s.repo.LatestRealname(ctx, userID)
	if errors.Is(err, infra.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return c.Status == contract.CertApproved, nil
}

// Extract 实现 trust.ReviewSubjectProvider（认证材料，一期实名无材料）。
func (s *Service) Extract(ctx context.Context, userID int64) (string, []string, error) {
	c, err := s.repo.LatestRealname(ctx, userID)
	if err != nil {
		return "", nil, err
	}
	var images []string
	for _, id := range c.MaterialIDs {
		images = append(images, fmt.Sprintf("%d", id))
	}
	return "", images, nil
}

// ApplyVerdict 实现 trust.ReviewSubjectProvider（平台认证审核结论落回，一期实名实时核验不涉及）。
func (s *Service) ApplyVerdict(_ context.Context, _ int64, _ bool, _ string) error { return nil }

// SubjectType 实现 trust.ReviewSubjectProvider。
func (s *Service) SubjectType() string { return "verify" }

func toContract(c infra.Cert) contract.Cert {
	out := contract.Cert{
		ID: c.ID, UserID: c.UserID, CertType: c.CertType, TargetOrgID: c.TargetOrgID,
		Status: c.Status, RejectReason: c.RejectReason, SubmittedAt: c.SubmitterAt, ReviewedAt: c.ReviewedAt,
	}
	return out
}
