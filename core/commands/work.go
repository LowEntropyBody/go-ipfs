package commands

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"text/tabwriter"

	bitswap "github.com/ipfs/go-bitswap"
	cmds "github.com/ipfs/go-ipfs-cmds"
	"github.com/ipfs/go-ipfs/core/commands/cmdenv"
	e "github.com/ipfs/go-ipfs/core/commands/e"
	corerepo "github.com/ipfs/go-ipfs/core/corerepo"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	path "github.com/ipfs/interface-go-ipfs-core/path"
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
	WorkLoad           int Workload score = sum(Files size)
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
		api, err := cmdenv.GetApi(env, req)
		if err != nil {
			return err
		}

		pinKeys := n.Pinning.RecursiveKeys()

		fileRootNodes := make([]BlockNode, len(pinKeys))
		for i, c := range pinKeys {
			fileRootNode := BlockNode{
				Hash: c.String(),
			}

			err = recursiveFillNode(&fileRootNode, api, req)

			if err != nil {
				return err
			}

			fileRootNodes[i] = fileRootNode
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

func recursiveFillNode(node *BlockNode, api coreiface.CoreAPI, req *cmds.Request) error {
	path := path.New(node.Hash)

	rp, err := api.ResolvePath(req.Context, path)
	if err != nil {
		return err
	}

	links, err := api.Object().Links(req.Context, rp)
	if err != nil {
		return err
	}

	if len(links) == 0 {
		return nil
	}

	dataIO, err := api.Object().Data(req.Context, path)
	if err != nil {
		return err
	}

	data, err := ioutil.ReadAll(dataIO)
	if err != nil {
		return err
	}

	node.Data = string(data)

	blockNodes := make([]BlockNode, len(links))

	for i, link := range links {
		blockNode := BlockNode{
			Hash: link.Cid.String(),
		}

		recursiveFillNode(&blockNode, api, req)

		blockNodes[i] = blockNode
	}

	node.BlockNodes = blockNodes

	return nil
}
