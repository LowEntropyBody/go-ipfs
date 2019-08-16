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

type BlockNode struct {
	Hash       string
	Size       int64
	BlockNodes []BlockNode
	Data       string
}

type WorkOutput struct {
	RepoSize          int64
	DeltaRepoSize     int64
	SendDataSize      int64
	DeltaSendDataSize int64
	FileRootNodes     []BlockNode
	WorkLoad          int64
}

var oldWorkOutput *WorkOutput

var WorkCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Show ipfs node workload info.",
		ShortDescription: `
EXAMPLE:
	ipfs work
Output:
	RepoSize           int Size in bytes that the repo is currently taking.
	DeltaRepoSize      int Size in bytes that the change of repo size
	SendDataSize       int Size in bytes that the node upload.
	DeltaSendDataSize  int Size in bytes that the change of send data size
	FileRootNodes      File root node collection
	WorkLoad           int Workload score = RepoSize + 5 * (DeltaRepoSize + DeltaSendDataSize)
`,
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		// Get node
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

		// Bitswap info
		bs, ok := n.Exchange.(*bitswap.Bitswap)
		if !ok {
			return e.TypeErr(bs, n.Exchange)
		}

		bitswapStat, err := bs.Stat()
		if err != nil {
			return err
		}

		// Get file root nodes
		var fileRootNodes []BlockNode
		for _, c := range n.Pinning.RecursiveKeys() {
			fileRootNode := &BlockNode{
				Hash: c.String(),
			}

			fileRootNodes = append(fileRootNodes, *fileRootNode)
		}

		// Output
		if oldWorkOutput == nil {
			oldWorkOutput = &WorkOutput{
				RepoSize:          int64(repoStat.RepoSize),
				DeltaRepoSize:     0,
				SendDataSize:      int64(bitswapStat.DataSent),
				DeltaSendDataSize: 0,
				FileRootNodes:     fileRootNodes,
				WorkLoad:          int64(repoStat.RepoSize),
			}

			return cmds.EmitOnce(res, oldWorkOutput)
		}

		newWorkOutput := &WorkOutput{
			RepoSize:          int64(repoStat.RepoSize),
			DeltaRepoSize:     int64(repoStat.RepoSize) - oldWorkOutput.RepoSize,
			SendDataSize:      int64(bitswapStat.DataSent),
			DeltaSendDataSize: int64(bitswapStat.DataSent) - oldWorkOutput.SendDataSize,
			FileRootNodes:     fileRootNodes,
			WorkLoad:          int64(repoStat.RepoSize) + 5*((int64(repoStat.RepoSize)-oldWorkOutput.RepoSize)+(int64(bitswapStat.DataSent)-oldWorkOutput.SendDataSize)),
		}

		oldWorkOutput = newWorkOutput
		return cmds.EmitOnce(res, newWorkOutput)
	},
	Type: &WorkOutput{},
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeTypedEncoder(func(req *cmds.Request, w io.Writer, out *WorkOutput) error {
			wtr := tabwriter.NewWriter(w, 0, 0, 1, ' ', 0)
			defer wtr.Flush()

			fmt.Fprintf(wtr, "%s:\t%d\n", "RepoSize", out.RepoSize)
			fmt.Fprintf(wtr, "%s:\t%d\n", "DeltaRepoSize", out.DeltaRepoSize)
			fmt.Fprintf(wtr, "%s:\t%d\n", "SendDataSize", out.SendDataSize)
			fmt.Fprintf(wtr, "%s:\t%d\n", "DeltaSendDataSize", out.DeltaSendDataSize)
			fmt.Fprintf(wtr, "%s:\t%d\n", "Score", out.WorkLoad)
			return nil
		}),
	},
}
