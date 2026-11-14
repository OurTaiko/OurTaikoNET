package multiplay

import (
	"cmp"
	"slices"

	pb "ourtaiko.dev/net/internal/multiplay/multiplaypb"
)

func clamp(v, low, high int32) int32 { return min(high, max(low, v)) }

// sanitizeStatus keeps what a client reports in range, so a broken client cannot break the
// results. Nil is a status of zeros. The answer is a new message: what was sent to players is
// never changed afterwards.
func sanitizeStatus(s *pb.PlayerStatus) *pb.PlayerStatus {
	const count = 100_000
	return &pb.PlayerStatus{
		Score:       clamp(s.GetScore(), 0, 100_000_000),
		Combo:       clamp(s.GetCombo(), 0, count),
		MaxCombo:    clamp(s.GetMaxCombo(), 0, count),
		Good:        clamp(s.GetGood(), 0, count),
		Ok:          clamp(s.GetOk(), 0, count),
		Bad:         clamp(s.GetBad(), 0, count),
		Drumroll:    clamp(s.GetDrumroll(), 0, count),
		Gauge:       clamp(s.GetGauge(), 0, 10_000),
		MusicTimeMs: clamp(s.GetMusicTimeMs(), 0, 3_600_000),
	}
}

// played is one player of a round, as the result ranks them.
type played struct {
	userID, nickname string
	avatarURL        string
	slot             int32
	course           string
	level            int32
	status           *pb.PlayerStatus
	cleared          bool
	// disconnected: the player did not finish (left, lost connection, or was still playing
	// when the round was ended).
	disconnected bool
}

// rankResults orders the players of a round: those who finished first, then by score, 良,
// max combo and gauge; players equal in all of them keep the order given. The top finisher
// is the MVP, or the SUPER STAR when its score is at least 1.3 times the second finisher's;
// there is no award with fewer than two finishers or a top score of zero.
func rankResults(players []played, round int32, song *Song) *pb.PlayResult {
	sorted := slices.Clone(players)
	slices.SortStableFunc(sorted, func(a, b played) int {
		if a.disconnected != b.disconnected {
			if a.disconnected {
				return 1
			}
			return -1
		}
		return cmp.Or(
			cmp.Compare(b.status.GetScore(), a.status.GetScore()),
			cmp.Compare(b.status.GetGood(), a.status.GetGood()),
			cmp.Compare(b.status.GetMaxCombo(), a.status.GetMaxCombo()),
			cmp.Compare(b.status.GetGauge(), a.status.GetGauge()),
		)
	})
	finishers := 0
	for _, p := range sorted {
		if !p.disconnected {
			finishers++
		}
	}
	top := pb.Award_NO_AWARD
	if finishers >= 2 && sorted[0].status.GetScore() > 0 {
		top = pb.Award_MVP
		if int64(sorted[0].status.GetScore())*10 >= int64(sorted[1].status.GetScore())*13 {
			top = pb.Award_SUPER_STAR
		}
	}
	result := &pb.PlayResult{Round: round}
	if song != nil {
		result.Source, result.ChartId, result.Title = song.Source, song.ChartID, song.Title
	}
	for i, p := range sorted {
		s := p.status
		award := pb.Award_NO_AWARD
		if i == 0 && !p.disconnected {
			award = top
		}
		fullCombo := !p.disconnected && s.GetBad() == 0 && s.GetGood()+s.GetOk() > 0
		result.TeamScore += int64(s.GetScore())
		result.Players = append(result.Players, &pb.PlayerResult{
			UserId: p.userID, Nickname: p.nickname, AvatarUrl: p.avatarURL, Course: p.course, Level: p.level, Status: s,
			Rank: int32(i + 1), Award: award, Cleared: p.cleared && !p.disconnected, Disconnected: p.disconnected,
			FullCombo: fullCombo, AllGood: fullCombo && s.GetOk() == 0, Slot: p.slot,
		})
	}
	return result
}
