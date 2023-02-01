pub mod ast;
pub mod compiler;
pub mod format;
pub mod parser;
pub mod vm;

mod tests;

#[derive(Debug)]
pub enum Error {
    CompilerError(compiler::Error),
    ParserError(parser::Error),
    RuntimeError(vm::Error),
    IOError(std::io::Error),
}

impl std::fmt::Display for Error {
    fn fmt(&self, f: &mut std::fmt::Formatter) -> std::fmt::Result {
        match self {
            Error::ParserError(e) => write!(f, "Parsing Error: {:#?}", e),
            Error::CompilerError(e) => write!(f, "Compiler Error: {:#?}", e),
            Error::RuntimeError(e) => write!(f, "Runtime Error: {:#?}", e),
            Error::IOError(e) => write!(f, "Input/Output Error: {:#?}", e),
        }
    }
}

impl std::error::Error for Error {}

impl From<std::io::Error> for Error {
    fn from(e: std::io::Error) -> Self {
        Error::IOError(e)
    }
}

impl From<compiler::Error> for Error {
    fn from(e: compiler::Error) -> Self {
        Error::CompilerError(e)
    }
}

impl From<parser::Error> for Error {
    fn from(e: parser::Error) -> Self {
        Error::ParserError(e)
    }
}

impl From<vm::Error> for Error {
    fn from(e: vm::Error) -> Self {
        Error::RuntimeError(e)
    }
}
