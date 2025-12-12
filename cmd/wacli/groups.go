package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/steipete/wacli/internal/out"
	"github.com/steipete/wacli/internal/store"
	"github.com/steipete/wacli/internal/wa"
	"go.mau.fi/whatsmeow/types"
)

func newGroupsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "groups",
		Short: "Group management",
	}
	cmd.AddCommand(newGroupsListCmd(flags))
	cmd.AddCommand(newGroupsRefreshCmd(flags))
	cmd.AddCommand(newGroupsInfoCmd(flags))
	cmd.AddCommand(newGroupsRenameCmd(flags))
	cmd.AddCommand(newGroupsParticipantsCmd(flags))
	cmd.AddCommand(newGroupsInviteCmd(flags))
	cmd.AddCommand(newGroupsJoinCmd(flags))
	cmd.AddCommand(newGroupsLeaveCmd(flags))
	return cmd
}

func newGroupsRefreshCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "refresh",
		Short: "Fetch joined groups (live) and update local DB",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := withTimeout(context.Background(), flags)
			defer cancel()

			a, lk, err := newApp(ctx, flags, true, false)
			if err != nil {
				return err
			}
			defer closeApp(a, lk)

			if err := a.EnsureAuthed(); err != nil {
				return err
			}
			if err := a.Connect(ctx, false, nil); err != nil {
				return err
			}

			gs, err := a.WA().GetJoinedGroups(ctx)
			if err != nil {
				return err
			}
			for _, g := range gs {
				if g == nil {
					continue
				}
				_ = persistGroupInfo(a.DB(), g)
				_ = a.DB().UpsertChat(g.JID.String(), "group", g.GroupName.Name, time.Now())
			}

			if flags.asJSON {
				return out.WriteJSON(os.Stdout, map[string]any{"groups": len(gs)})
			}
			fmt.Fprintf(os.Stdout, "Imported %d groups.\n", len(gs))
			return nil
		},
	}
	return cmd
}

func newGroupsListCmd(flags *rootFlags) *cobra.Command {
	var query string
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List known groups (from local DB; run sync to populate)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := withTimeout(context.Background(), flags)
			defer cancel()

			a, lk, err := newApp(ctx, flags, false, false)
			if err != nil {
				return err
			}
			defer closeApp(a, lk)

			gs, err := a.DB().ListGroups(query, limit)
			if err != nil {
				return err
			}
			if flags.asJSON {
				return out.WriteJSON(os.Stdout, gs)
			}

			w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tJID\tCREATED")
			for _, g := range gs {
				name := g.Name
				if name == "" {
					name = g.JID
				}
				fmt.Fprintf(w, "%s\t%s\t%s\n", truncate(name, 40), g.JID, g.CreatedAt.Local().Format("2006-01-02"))
			}
			_ = w.Flush()
			return nil
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "search query")
	cmd.Flags().IntVar(&limit, "limit", 50, "limit")
	return cmd
}

func newGroupsInfoCmd(flags *rootFlags) *cobra.Command {
	var jidStr string
	cmd := &cobra.Command{
		Use:   "info",
		Short: "Fetch group info (live) and update local DB",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(jidStr) == "" {
				return fmt.Errorf("--jid is required")
			}
			ctx, cancel := withTimeout(context.Background(), flags)
			defer cancel()

			a, lk, err := newApp(ctx, flags, true, false)
			if err != nil {
				return err
			}
			defer closeApp(a, lk)

			if err := a.EnsureAuthed(); err != nil {
				return err
			}
			if err := a.Connect(ctx, false, nil); err != nil {
				return err
			}

			gjid, err := types.ParseJID(jidStr)
			if err != nil {
				return err
			}
			info, err := a.WA().GetGroupInfo(ctx, gjid)
			if err != nil {
				return err
			}
			if info != nil {
				_ = persistGroupInfo(a.DB(), info)
			}

			if flags.asJSON {
				return out.WriteJSON(os.Stdout, info)
			}

			fmt.Fprintf(os.Stdout, "JID: %s\nName: %s\nOwner: %s\nCreated: %s\nParticipants: %d\n",
				info.JID.String(),
				info.GroupName.Name,
				info.OwnerJID.String(),
				info.GroupCreated.Local().Format(time.RFC3339),
				len(info.Participants),
			)
			return nil
		},
	}
	cmd.Flags().StringVar(&jidStr, "jid", "", "group JID (…@g.us)")
	return cmd
}

func newGroupsRenameCmd(flags *rootFlags) *cobra.Command {
	var jidStr string
	var name string
	cmd := &cobra.Command{
		Use:   "rename",
		Short: "Rename group",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(jidStr) == "" || strings.TrimSpace(name) == "" {
				return fmt.Errorf("--jid and --name are required")
			}
			ctx, cancel := withTimeout(context.Background(), flags)
			defer cancel()

			a, lk, err := newApp(ctx, flags, true, false)
			if err != nil {
				return err
			}
			defer closeApp(a, lk)

			if err := a.EnsureAuthed(); err != nil {
				return err
			}
			if err := a.Connect(ctx, false, nil); err != nil {
				return err
			}

			gjid, err := types.ParseJID(jidStr)
			if err != nil {
				return err
			}
			if err := a.WA().SetGroupName(ctx, gjid, name); err != nil {
				return err
			}
			if info, err := a.WA().GetGroupInfo(ctx, gjid); err == nil && info != nil {
				_ = persistGroupInfo(a.DB(), info)
			}
			if flags.asJSON {
				return out.WriteJSON(os.Stdout, map[string]any{"jid": gjid.String(), "name": name})
			}
			fmt.Fprintln(os.Stdout, "OK")
			return nil
		},
	}
	cmd.Flags().StringVar(&jidStr, "jid", "", "group JID (…@g.us)")
	cmd.Flags().StringVar(&name, "name", "", "new name")
	return cmd
}

func newGroupsParticipantsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "participants",
		Short: "Manage group participants",
	}
	cmd.AddCommand(newGroupsParticipantsActionCmd(flags, "add"))
	cmd.AddCommand(newGroupsParticipantsActionCmd(flags, "remove"))
	cmd.AddCommand(newGroupsParticipantsActionCmd(flags, "promote"))
	cmd.AddCommand(newGroupsParticipantsActionCmd(flags, "demote"))
	return cmd
}

func newGroupsParticipantsActionCmd(flags *rootFlags, action string) *cobra.Command {
	var group string
	var users []string
	cmd := &cobra.Command{
		Use:   action,
		Short: action + " participants",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(group) == "" || len(users) == 0 {
				return fmt.Errorf("--jid and at least one --user are required")
			}
			ctx, cancel := withTimeout(context.Background(), flags)
			defer cancel()

			a, lk, err := newApp(ctx, flags, true, false)
			if err != nil {
				return err
			}
			defer closeApp(a, lk)

			if err := a.EnsureAuthed(); err != nil {
				return err
			}
			if err := a.Connect(ctx, false, nil); err != nil {
				return err
			}

			gjid, err := types.ParseJID(group)
			if err != nil {
				return err
			}
			var jids []types.JID
			for _, u := range users {
				j, err := wa.ParseUserOrJID(u)
				if err != nil {
					return err
				}
				jids = append(jids, j)
			}

			updated, err := a.WA().UpdateGroupParticipants(ctx, gjid, jids, wa.GroupParticipantAction(action))
			if err != nil {
				return err
			}
			if info, err := a.WA().GetGroupInfo(ctx, gjid); err == nil && info != nil {
				_ = persistGroupInfo(a.DB(), info)
			}

			if flags.asJSON {
				return out.WriteJSON(os.Stdout, updated)
			}
			fmt.Fprintln(os.Stdout, "OK")
			return nil
		},
	}
	cmd.Flags().StringVar(&group, "jid", "", "group JID (…@g.us)")
	cmd.Flags().StringSliceVar(&users, "user", nil, "user phone number or JID (repeatable)")
	return cmd
}

func newGroupsInviteCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "invite",
		Short: "Manage group invite links",
	}
	cmd.AddCommand(newGroupsInviteLinkCmd(flags))
	return cmd
}

func newGroupsInviteLinkCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "link",
		Short: "Get or revoke invite links",
	}
	cmd.AddCommand(newGroupsInviteLinkGetCmd(flags))
	cmd.AddCommand(newGroupsInviteLinkRevokeCmd(flags))
	return cmd
}

func newGroupsInviteLinkGetCmd(flags *rootFlags) *cobra.Command {
	var jidStr string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get invite link",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(jidStr) == "" {
				return fmt.Errorf("--jid is required")
			}
			ctx, cancel := withTimeout(context.Background(), flags)
			defer cancel()

			a, lk, err := newApp(ctx, flags, true, false)
			if err != nil {
				return err
			}
			defer closeApp(a, lk)

			if err := a.EnsureAuthed(); err != nil {
				return err
			}
			if err := a.Connect(ctx, false, nil); err != nil {
				return err
			}
			gjid, err := types.ParseJID(jidStr)
			if err != nil {
				return err
			}
			link, err := a.WA().GetGroupInviteLink(ctx, gjid, false)
			if err != nil {
				return err
			}
			if flags.asJSON {
				return out.WriteJSON(os.Stdout, map[string]any{"jid": gjid.String(), "link": link})
			}
			fmt.Fprintln(os.Stdout, link)
			return nil
		},
	}
	cmd.Flags().StringVar(&jidStr, "jid", "", "group JID (…@g.us)")
	return cmd
}

func newGroupsInviteLinkRevokeCmd(flags *rootFlags) *cobra.Command {
	var jidStr string
	cmd := &cobra.Command{
		Use:   "revoke",
		Short: "Revoke/reset invite link",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(jidStr) == "" {
				return fmt.Errorf("--jid is required")
			}
			ctx, cancel := withTimeout(context.Background(), flags)
			defer cancel()

			a, lk, err := newApp(ctx, flags, true, false)
			if err != nil {
				return err
			}
			defer closeApp(a, lk)

			if err := a.EnsureAuthed(); err != nil {
				return err
			}
			if err := a.Connect(ctx, false, nil); err != nil {
				return err
			}
			gjid, err := types.ParseJID(jidStr)
			if err != nil {
				return err
			}
			link, err := a.WA().GetGroupInviteLink(ctx, gjid, true)
			if err != nil {
				return err
			}
			if flags.asJSON {
				return out.WriteJSON(os.Stdout, map[string]any{"jid": gjid.String(), "link": link, "revoked": true})
			}
			fmt.Fprintln(os.Stdout, link)
			return nil
		},
	}
	cmd.Flags().StringVar(&jidStr, "jid", "", "group JID (…@g.us)")
	return cmd
}

func newGroupsJoinCmd(flags *rootFlags) *cobra.Command {
	var code string
	cmd := &cobra.Command{
		Use:   "join",
		Short: "Join group by invite code",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(code) == "" {
				return fmt.Errorf("--code is required")
			}
			ctx, cancel := withTimeout(context.Background(), flags)
			defer cancel()

			a, lk, err := newApp(ctx, flags, true, false)
			if err != nil {
				return err
			}
			defer closeApp(a, lk)

			if err := a.EnsureAuthed(); err != nil {
				return err
			}
			if err := a.Connect(ctx, false, nil); err != nil {
				return err
			}
			jid, err := a.WA().JoinGroupWithLink(ctx, code)
			if err != nil {
				return err
			}
			if info, err := a.WA().GetGroupInfo(ctx, jid); err == nil && info != nil {
				_ = persistGroupInfo(a.DB(), info)
			}
			if flags.asJSON {
				return out.WriteJSON(os.Stdout, map[string]any{"jid": jid.String(), "joined": true})
			}
			fmt.Fprintf(os.Stdout, "Joined: %s\n", jid.String())
			return nil
		},
	}
	cmd.Flags().StringVar(&code, "code", "", "invite code (from link)")
	return cmd
}

func newGroupsLeaveCmd(flags *rootFlags) *cobra.Command {
	var jidStr string
	cmd := &cobra.Command{
		Use:   "leave",
		Short: "Leave a group",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(jidStr) == "" {
				return fmt.Errorf("--jid is required")
			}
			ctx, cancel := withTimeout(context.Background(), flags)
			defer cancel()

			a, lk, err := newApp(ctx, flags, true, false)
			if err != nil {
				return err
			}
			defer closeApp(a, lk)

			if err := a.EnsureAuthed(); err != nil {
				return err
			}
			if err := a.Connect(ctx, false, nil); err != nil {
				return err
			}
			gjid, err := types.ParseJID(jidStr)
			if err != nil {
				return err
			}
			if err := a.WA().LeaveGroup(ctx, gjid); err != nil {
				return err
			}
			if flags.asJSON {
				return out.WriteJSON(os.Stdout, map[string]any{"jid": gjid.String(), "left": true})
			}
			fmt.Fprintln(os.Stdout, "OK")
			return nil
		},
	}
	cmd.Flags().StringVar(&jidStr, "jid", "", "group JID (…@g.us)")
	return cmd
}

func persistGroupInfo(db *store.DB, info *types.GroupInfo) error {
	if info == nil {
		return nil
	}
	if err := db.UpsertGroup(info.JID.String(), info.GroupName.Name, info.OwnerJID.String(), info.GroupCreated); err != nil {
		return err
	}
	var ps []store.GroupParticipant
	for _, p := range info.Participants {
		role := "member"
		if p.IsSuperAdmin {
			role = "superadmin"
		} else if p.IsAdmin {
			role = "admin"
		}
		ps = append(ps, store.GroupParticipant{
			GroupJID: info.JID.String(),
			UserJID:  p.JID.String(),
			Role:     role,
		})
	}
	return db.ReplaceGroupParticipants(info.JID.String(), ps)
}
