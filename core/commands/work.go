package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"strings"
	"text/tabwriter"

	cid "github.com/ipfs/go-cid"
	coreapi "github.com/ipfs/go-ipfs/core/coreapi"
	ipld "github.com/ipfs/go-ipld-format"
	dag "github.com/ipfs/go-merkledag"

	cmds "github.com/ipfs/go-ipfs-cmds"
	"github.com/ipfs/go-ipfs/core/commands/cmdenv"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	path "github.com/ipfs/interface-go-ipfs-core/path"
)

const offlineWorkErrorMessage = `'ipfs work' currently cannot query information without a running daemon; we are working to fix this.
In the meantime, if you want to query workload using 'ipfs work',
please run the daemon:

    ipfs daemon &
    ipfs work
`

type Node struct {
	Hash   string
	Links  []string
	Name   string
	Data   string
	Size   int
	IsLeaf int
	IsRoot int
}

type WorkOutput struct {
	Nodes []Node
}

var oldWorkOutput *WorkOutput

var WorkCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Show ipfs node workload info.",
		ShortDescription: `
EXAMPLE:
	ipfs work
Output:
    Nodes        Node collection
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

		// Get file root nodes
		api, err := cmdenv.GetApi(env, req)
		if err != nil {
			return err
		}

		nodes := make(map[string]Node)

		for _, key := range n.Pinning.RecursiveKeys() {
			recursiveFillNode(nodes, key.String(), 1, api, req)
			if err != nil {
				return err
			}
		}

		// Output
		nodeValues := make([]Node, 0)
		for _, value := range nodes {
			nodeValues = append(nodeValues, value)
		}

		return cmds.EmitOnce(res, &WorkOutput{
			Nodes: nodeValues,
		})
	},
	Type: &WorkOutput{},
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeTypedEncoder(func(req *cmds.Request, w io.Writer, out *WorkOutput) error {
			wtr := tabwriter.NewWriter(w, 0, 0, 1, ' ', 0)
			defer wtr.Flush()

			outString, err := json.Marshal(*out)
			if err != nil {
				return err
			}

			fmt.Fprintf(wtr, "%s\n", outString)
			return nil
		}),
	},
}

func recursiveFillNode(nodes map[string]Node, hash string, isRoot int, api coreiface.CoreAPI, req *cmds.Request) error {
	if _, ok := nodes[hash]; ok {
		return nil
	}

	path := path.New(hash)

	nd, err := api.Object().Get(req.Context, path)
	if err != nil {
		return err
	}

	node := Node{
		Hash:   hash,
		IsRoot: isRoot,
		IsLeaf: 0,
		Links:  make([]string, len(nd.Links())),
	}

	for i, link := range nd.Links() {
		node.Links[i] = link.Cid.String()
		recursiveFillNode(nodes, link.Cid.String(), 0, api, req)
	}

	stat, err := nd.Stat()
	if err != nil {
		return err
	}

	node.Size = stat.BlockSize

	if stat.NumLinks == 0 {
		node.IsLeaf = 1
		nodes[hash] = node
		return nil
	}

	r, err := api.Object().Data(req.Context, path)
	if err != nil {
		return err
	}

	data, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}

	node.Data = string(data)
	nodes[hash] = node
	return nil
}

// For future work
func testNodeToBlock() error {
	data := "{\"Data\": \"another\",\"Links\": [ {\"Name\": \"some link\",\"Hash\": \"QmXg9Pp2ytZ14xgmQjYEiHjVjMFXzCVVEcRTWJBmLgR39V\",\"Size\": 8} ]}"
	node := new(coreapi.Node)
	decoder := json.NewDecoder(strings.NewReader(data))

	decoder.DisallowUnknownFields()
	decoder.Decode(node)

	dagnode, err := deserializeNode(node)
	if err != nil {
		return err
	}

	outString, err := json.Marshal(dagnode)
	if err != nil {
		return err
	}

	fmt.Println(string(outString))
	fmt.Println(dagnode.Cid())
	fmt.Println(string(dagnode.RawData()))

	return nil
}

func deserializeNode(nd *coreapi.Node) (*dag.ProtoNode, error) {
	dagnode := new(dag.ProtoNode)
	dagnode.SetData([]byte(nd.Data))

	links := make([]*ipld.Link, len(nd.Links))
	for i, link := range nd.Links {
		c, err := cid.Decode(link.Hash)
		if err != nil {
			return nil, err
		}
		links[i] = &ipld.Link{
			Name: link.Name,
			Size: link.Size,
			Cid:  c,
		}
	}
	dagnode.SetLinks(links)

	return dagnode, nil
}
