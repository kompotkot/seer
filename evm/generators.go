// - [ ] Working CLI generation for deployment, calls, transactions against a contract given its ABI and bytecode.
// - [ ] Generated code has a header comment explaining that code is generated by seer, modify at your own risk, etc.
// - [ ] Generated CLI contains a command to crawl and parse contract events.

package evm

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"text/template"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/iancoleman/strcase"
)

// GenerateTypes generates Go bindings to an Ethereum contract ABI (or union of such). This functionality
// is roughly equivalent to that provided by the `abigen` tool provided by go-ethereum:
// https://github.com/ethereum/go-ethereum/tree/master/cmd/abigen
// Under the hood, GenerateTypes uses the Bind method implemented in go-ethereum in a manner similar to
// abigen. It just offers a simpler call signature that makes more sense for users of seer.
//
// Arguments:
//  1. structName: The name of the generated Go struct that will represent this contract.
//  2. abi: The bytes representing the contract's ABI.
//  3. bytecode: The bytes representing the contract's bytecode. If this is provided, a "deploy" method
//     will be generated. If it is not provided, no such method will be generated.
//  4. packageName: If this is provided, the generated code will contain a package declaration of this name.
func GenerateTypes(structName string, abi []byte, bytecode []byte, packageName string) (string, error) {
	return bind.Bind([]string{structName}, []string{string(abi)}, []string{string(bytecode)}, []map[string]string{}, packageName, bind.LangGo, map[string]string{}, map[string]string{})
}

type cliParams struct {
	StructName string
}

// AddCLI adds CLI code (using github.com/spf13/cobra command-line framework) for code generated by the
// GenerateTypes function. The output of this function *contains* the input, with enrichments (some of
// then inline). It should not be concatenated with the output of GenerateTypes, but rather be used as
// part of a chain.
func AddCLI(sourceCode, structName string) (string, error) {
	fileset := token.NewFileSet()
	filename := ""
	sourceAST, sourceASTErr := parser.ParseFile(fileset, filename, sourceCode, parser.ParseComments)
	if sourceASTErr != nil {
		return "", sourceASTErr
	}

	deployer := fmt.Sprintf("Deploy%s", structName)
	callerReceiver := fmt.Sprintf("%sCallerSession", structName)
	transactorReceiver := fmt.Sprintf("%sTransactorSession", structName)

	var deployMethod *ast.FuncDecl
	structViewMethods := map[string]*ast.FuncDecl{}
	structTransferMethods := map[string]*ast.FuncDecl{}

	ast.Inspect(sourceAST, func(node ast.Node) bool {
		switch t := node.(type) {
		case *ast.GenDecl:
			// Add additional imports:
			// - os
			// - github.com/spf13/cobra
			if t.Tok == token.IMPORT {
				t.Specs = append(t.Specs, &ast.ImportSpec{Path: &ast.BasicLit{Value: `"os"`}}, &ast.ImportSpec{Path: &ast.BasicLit{Value: `"github.com/spf13/cobra"`}})
			}
			return true
		case *ast.FuncDecl:
			if t.Recv != nil {
				receiverName := t.Recv.List[0].Type.(*ast.StarExpr).X.(*ast.Ident).Name
				if receiverName == callerReceiver {
					structViewMethods[t.Name.Name] = t
				} else if receiverName == transactorReceiver {
					structTransferMethods[t.Name.Name] = t
				}
			} else {
				if t.Name.Name == deployer {
					deployMethod = t
				}
			}
			return false
		default:
			return true
		}
	})

	var codeBytes bytes.Buffer
	printer.Fprint(&codeBytes, fileset, sourceAST)
	code := codeBytes.String()

	if deployMethod != nil {
		fmt.Printf("Deploy: %s\n", deployMethod.Name.Name)
	}

	fmt.Println("View methods:")
	for methodName, _ := range structViewMethods {
		fmt.Printf("- %s\n", methodName)
	}

	fmt.Println("Transfer methods:")
	for methodName, _ := range structTransferMethods {
		fmt.Printf("- %s\n", methodName)
	}

	templateFuncs := map[string]any{
		"KebabCase":      strcase.ToKebab,
		"ScreamingSnake": strcase.ToScreamingSnake,
	}

	cliTemplate, cliTemplateParseErr := template.New("cli").Funcs(templateFuncs).Parse(CLICodeTemplate)
	if cliTemplateParseErr != nil {
		return code, cliTemplateParseErr
	}

	params := cliParams{StructName: structName}
	var b bytes.Buffer
	templateErr := cliTemplate.Execute(&b, params)
	if templateErr != nil {
		return code, templateErr
	}

	return code + "\n\n" + b.String(), nil
}

var CLICodeTemplate string = `
var ErrNoRPCURL error = errors.New("no RPC URL provided -- please pass an RPC URL from the command line or set the {{(ScreamingSnake .StructName)}}_RPC_URL environment variable")

// Generates an Ethereum client to the JSONRPC API at the given URL. If rpcURL is empty, then it
// attempts to read the RPC URL from the {{(ScreamingSnake .StructName)}}_RPC_URL environment variable. If that is empty,
// too, then it returns an error.
func NewClient(rpcURL string) (*ethclient.Client, error) {
	if rpcURL == "" {
		rpcURL = os.Getenv("{{(ScreamingSnake .StructName)}}_RPC_URL")
	}

	if rpcURL == "" {
		return nil, ErrNoRPCURL
	}

	client, err := ethclient.Dial(rpcURL)
	return client, err
}

// Creates a new context to be used when interacting with the chain client.
func NewChainContext(timeout uint) (context.Context, context.CancelFunc) {
	baseCtx := context.Background()
	parsedTimeout := time.Duration(timeout) * time.Second
	ctx, cancel := context.WithTimeout(baseCtx, parsedTimeout)
	return ctx, cancel
}

// Unlocks a key from a keystore (byte contents of a keystore file) with the given password.
func UnlockKeystore(keystoreData []byte, password string) (*keystore.Key, error) {
	key, err := keystore.DecryptKey(keystoreData, password)
	return key, err
}

// Loads a key from file, prompting the user for the password if it is not provided as a function argument.
func KeyFromFile(keystoreFile string, password string) (*keystore.Key, error) {
	var emptyKey *keystore.Key
	keystoreContent, readErr := os.ReadFile(keystoreFile)
	if readErr != nil {
		return emptyKey, readErr
	}

	// If password is "", prompt user for password.
	if password == "" {
		fmt.Printf("Please provide a password for keystore (%s): ", keystoreFile)
		passwordRaw, inputErr := term.ReadPassword(int(os.Stdin.Fd()))
		if inputErr != nil {
			return emptyKey, fmt.Errorf("error reading password: %s", inputErr.Error())
		}
		fmt.Print("\n")
		password = string(passwordRaw)
	}

	key, err := UnlockKeystore(keystoreContent, password)
	return key, err
}

// This method is used to set the parameters on a transaction from command line arguments (represented as
// strings).
func SetTransactionParametersFromArgs(opts *bind.TransactOpts, nonce, value, gasPrice, maxFeePerGas, maxPriorityFeePerGas string, gasLimit uint64, noSend bool) {
	if nonce != "" {
		opts.Nonce = new(big.Int)
		opts.Nonce.SetString(nonce, 0)
	}

	if value != "" {
		opts.Value = new(big.Int)
		opts.Value.SetString(value, 0)
	}

	if gasPrice != "" {
		opts.GasPrice = new(big.Int)
		opts.GasPrice.SetString(gasPrice, 0)
	}

	if maxFeePerGas != "" {
		opts.GasFeeCap = new(big.Int)
		opts.GasFeeCap.SetString(maxFeePerGas, 0)
	}

	if maxPriorityFeePerGas != "" {
		opts.GasTipCap = new(big.Int)
		opts.GasTipCap.SetString(maxPriorityFeePerGas, 0)
	}

	if gasLimit != 0 {
		opts.GasLimit = gasLimit
	}

	opts.NoSend = noSend
}

func Create{{.StructName}}Command() *cobra.Command {
	// Command line settings for call methods
	var callBlockNumber string

	cmd := &cobra.Command{
		Use:  "{{(KebabCase .StructName)}}",
		Short: "Interact with the {{.StructName}} contract",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}

	cmd.SetOut(os.Stdout)

	DeployGroup := &cobra.Group{
		ID: "deploy", Title: "Commands which deploy contracts",
	}
	ViewGroup := &cobra.Group{
		ID: "view", Title: "Commands which view contract state",
	}
	TransactGroup := &cobra.Group{
		ID: "transact", Title: "Commands which submit transactions",
	}
	cmd.AddGroup(DeployGroup, ViewGroup, TransactGroup)

	return cmd
}
`
