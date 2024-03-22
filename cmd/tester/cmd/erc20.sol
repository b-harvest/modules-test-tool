import "@openzeppelin/contracts/token/ERC20/ERC20.sol";
import "@openzeppelin/contracts/token/ERC20/extensions/ERC20Burnable.sol";
import "@openzeppelin/contracts/security/Pausable.sol";
import "@openzeppelin/contracts/access/Ownable.sol";

contract DevToken is ERC20{
    constructor() ERC20("DevToken", "DVT"){
        _mint(msg.sender,1000*10**18);
    }
}
