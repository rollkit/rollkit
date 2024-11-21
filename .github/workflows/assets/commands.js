const commands = Object.freeze([
    {
      name: "Install Ignite",
      command: "curl https://get.ignite.com/cli@v28.5.3! | bash -- --no",
    },
    {
      name: "Scaffold Chain",
      command: "ignite scaffold chain gm --address-prefix gm --minimal --skip-proto",
    },
    {
      name: "cd gm",
      command: "cd gm",
    },
    {
      name: "Install Rollkit App",
      command: "ignite app install github.com/ignite/apps/rollkit@rollkit/v0.2.1",
    },
    {
      name: "Add Rollkit App",
      command: "ignite rollkit add",
    },
    {
      name: "Build Chain",
      command: "ignite chain build",
    },
    {
      name: "Init Rollkit",
      command: "ignite rollkit init",
    },
    {
      name: "Init Rollkit toml",
      command: "rollkit toml init",
    },
    {
      name: "Rollkit Start",
      command: "rollkit start --rollkit.aggregator --rollkit.sequencer_rollup_id gm --halt-height 10",
    },
  ]);
  export default commands;