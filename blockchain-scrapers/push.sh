#!/bin/bash
#set -x

function usage {
    echo "Usage: ./push.sh [-bdr] <service|all>"
    echo "  Each flag corresponds to an operation and takes a single argument. More than one operation can be run in the same command. The supported flags/operations are:"
    echo ""
    echo "  -b      builds an image for a service (builds everything when run with 'all'"
    echo "  -d      deploys a service in a stack with its name (deploys every service in a stack named 'all' when run with 'all'"
    echo "  -r      removes a service (or every service if run with 'all')"
    echo "  -h      display this usage guide"
    exit 1
}

function error {
	RED='\033[0;31m'
	NC='\033[0m'
	echo -e "${RED}Error: ${NC}$1" 1>&3
}

function deploy_stack {
	echo "Deploying stack '$1'..." 1>&3

	case $1 in
		# deploy every service in a stack called 'all'
		all)
			find $blockchain_dir -name docker-compose-*.yml -exec docker stack deploy -c {} $1 \;
			echo "Finished deployment" 1>&3
			;;

		# deploy a service in a stack with its name
		*)
			(docker stack deploy -c $(find $blockchain_dir -name docker-compose-$1.yml) $1 &&
				echo "Finished deployment" 1>&3) || error "Unknown service '$1'"
			;;
	esac
}

function build_service {
	echo "Building service '$1'..." 1>&3

	case $1 in
		# build every service
		all) 
			find $blockchain_dir -name Dockerfile* -exec sh -c "docker build -f {} -t $DOCKER_HUB_LOGIN/blockchain-scrapers_$(sed -n -e 's/^.*-//p' {}) $GOPATH" \;
			echo "Finished build" 1>&3			
			;;

		# build a particular service
		*) 
			(docker build -f $(find $blockchain_dir -name Dockerfile-$1) -t $DOCKER_HUB_LOGIN/blockchain-scrapers_$1 $GOPATH && 
				echo "Finished build" 1>&3) || error "Unknown service '$1'"
			;;
	esac
}

function remove_stack {
	echo "Removing stack '$1'..." 1>&3
	
	# remove a stack
	if [[ ! -z $(docker stack ls | grep "$1") ]]; then
		docker stack rm $1
		echo "Finished removal" 1>&3
	else
		error "Stack isn't deployed"
	fi
}

# if one of the commands is help, display only that
if [[ $@ == *"-h"* ]]; then
	usage
	exit 1
fi

# create necessary volumes 
blockchain_dir=$GOPATH/src/github.com/diadata-org/api-golang/blockchain-scrapers/blockchains
sudo mkdir -p $HOME/srv/bitcoin $HOME/srv/geth $HOME/srv/monero $HOME/srv/litecoin $HOME/srv/cardano $HOME/srv/bitcoin-cash $HOME/srv/neo 
sudo chmod -R 777 $HOME/srv

unset deploy
unset build
unset remove

# parse input
while getopts "d:b:r:" opt; do
	case $opt in
		d)
			deploy=$OPTARG
			;;

		b)
			build=$OPTARG
			;;

		r)
			remove=$OPTARG
			;;

		*)
			error "Unknown operation '$opt'"
	esac
done

# silence the output of every command
exec 3>&1 4>&2
{
	if [[ ! -z ${build} ]]; then
		build_service $build
	fi

	if [[ ! -z ${deploy} ]]; then
		deploy_stack $deploy
	fi

	if [[ ! -z ${remove} ]]; then
		remove_stack $remove
	fi

} &> /dev/null