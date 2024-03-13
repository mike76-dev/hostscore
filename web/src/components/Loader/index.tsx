import './index.css'

type LoaderProps = { darkMode: boolean }

const Loader = (props: LoaderProps) => {
	return (
		<div className={'loader' + (props.darkMode ? ' loader-dark' : '')}>
			<div></div>
			<div></div>
			<div></div>
			<div></div>
		</div>
	)
}

export default Loader