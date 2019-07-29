package commands

import (
	"errors"
	"fmt"
	"io"
	"text/tabwriter"

	bitswap "github.com/ipfs/go-bitswap"
	cmds "github.com/ipfs/go-ipfs-cmds"
	"github.com/ipfs/go-ipfs/core/commands/cmdenv"
	e "github.com/ipfs/go-ipfs/core/commands/e"
	corerepo "github.com/ipfs/go-ipfs/core/corerepo"
)

const offlineWorkErrorMessage = `'ipfs work' currently cannot query information without a running daemon; we are working to fix this.
In the meantime, if you want to query workload using 'ipfs work',
please run the daemon:

    ipfs daemon &
    ipfs work
`

type WorkOutput struct {
	RepoSize     uint64
	NumObjects   uint64
	SendDataSize uint64
	Score        uint64
}

var WorkCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Show ipfs node workload info.",
		ShortDescription: `
Prints out information about the specified peer.

EXAMPLE:
	ipfs work
Output:
	RepoSize        int Size in bytes that the repo is currently taking.
	NumObjects      int Number of objects in the local repo.
	SendDataSize    int Size in bytes that the node upload.
	Score           int workload score
`,
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}

		if !n.IsOnline {
			return errors.New(offlineWorkErrorMessage)
		}

		// Repo info
		repoStat, err := corerepo.RepoStat(req.Context, n)
		if err != nil {
			return err
		}

		bs, ok := n.Exchange.(*bitswap.Bitswap)
		if !ok {
			return e.TypeErr(bs, n.Exchange)
		}

		bitswapStat, err := bs.Stat()
		if err != nil {
			return err
		}

		return cmds.EmitOnce(res, WorkOutput{
			RepoSize:     repoStat.RepoSize,
			NumObjects:   repoStat.NumObjects,
			SendDataSize: bitswapStat.DataSent,
			Score:        5*repoStat.RepoSize + bitswapStat.DataSent,
		})
	},
	Type: WorkOutput{},
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeTypedEncoder(func(req *cmds.Request, w io.Writer, out *WorkOutput) error {
			wtr := tabwriter.NewWriter(w, 0, 0, 1, ' ', 0)
			defer wtr.Flush()

			fmt.Fprintf(wtr, "%s:\t%d\n", "RepoSize", &out.RepoSize)
			fmt.Fprintf(wtr, "%s:\t%d\n", "NumObjects", &out.NumObjects)
			fmt.Fprintf(wtr, "%s:\t%d\n", "SendDataSize", &out.SendDataSize)
			fmt.Fprintf(wtr, "%s:\t%d\n", "Score", &out.Score)
			return nil
		}),
	},
}
