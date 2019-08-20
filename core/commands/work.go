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
	Children         []Node
	Name, Hash, Data string
	Size             uint64
	IsLeaf           bool
}

type WorkOutput struct {
	FileRootNodes []Node
}

var oldWorkOutput *WorkOutput

var WorkCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Show ipfs node workload info.",
		ShortDescription: `
EXAMPLE:
	ipfs work
Output:
    FileRootNodes        File root node collection
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

		pinKeys := n.Pinning.RecursiveKeys()
		fileRootNodes := make([]Node, len(pinKeys))
		for i, c := range pinKeys {
			fileRootNodes[i] = Node{
				Hash: c.String(),
			}

			err = recursiveFillNode(&fileRootNodes[i], api, req)
			if err != nil {
				return err
			}
		}

		// Output
		return cmds.EmitOnce(res, &WorkOutput{
			FileRootNodes: fileRootNodes,
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

func recursiveFillNode(node *Node, api coreiface.CoreAPI, req *cmds.Request) error {
	path := path.New(node.Hash)

	nd, err := api.Object().Get(req.Context, path)
	if err != nil {
		return err
	}

	node.Children = make([]Node, len(nd.Links()))
	for i, link := range nd.Links() {
		node.Children[i] = Node{
			Hash: link.Cid.String(),
		}

		recursiveFillNode(&node.Children[i], api, req)
	}

	node.Size, err = nd.Size()
	if err != nil {
		return err
	}

	if len(nd.Links()) == 0 {
		node.IsLeaf = true
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
