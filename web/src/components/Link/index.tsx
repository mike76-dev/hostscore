import './index.css'

type LinkProps = {
    href: string,
    target?: string,
    caption: string,
    darkMode: boolean,
}

const Link = (props: LinkProps) => {
    return (
        <a
            className={'link' + (props.darkMode ? ' link-dark' : '')}
            href={props.href}
            target={props.target || ''}
            tabIndex={1}
        >
            {props.caption}
        </a>
    )
}

export default Link
